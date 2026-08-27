// © 2026 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/option"

	"github.com/platform-engineering-labs/formae/pkg/plugin"
	"github.com/platform-engineering-labs/oox/gcpname"
	oidcxgcp "github.com/platform-engineering-labs/oox/oidcx/gcp"
)

// Auth discriminator values, matching the Type field the Pkl schema renders
// on the nested Auth object.
const AuthTypeOidc = "Oidc"

// OidcDeps is owned by the Plugin instance, never process-global.
//
// Ownership is what scopes the token-source cache. A package-global cache
// keyed by provider and scopes could hand one installation's token source to
// another whose deps point at a different broker, which is a credential leak
// rather than a stale-cache annoyance.
//
// A Config with nil deps fails closed on Oidc auth: it never falls back to
// ambient credentials, because in a hosted installation "ambient" is whatever
// happens to be lying around in the agent's own environment.
type OidcDeps struct {
	// Source mints the OIDC identity tokens exchanged for Google credentials.
	Source plugin.OidcTokenSource

	// creds caches exchanged credentials per (provider, scopes) pair as plain
	// data: an access token and the instant it expires.
	//
	// Data, deliberately, and this is the whole point of the type. Minting an
	// identity token is an Ergo call, and Ergo permits one only from a process
	// in Init or Running - in practice, the PluginOperator actor whose handler
	// is on the stack right now. Anything cached here outlives that operator,
	// so it must not be able to reach one: an oauth2.TokenSource would, via
	// the context it captures at construction, and would then mint through an
	// actor that stopped running operations ago.
	//
	// Concurrent by necessity rather than by taste. Operators are one process
	// per operation and many run at once against this single plugin instance,
	// so every read and write here happens on a different actor's goroutine.
	creds sync.Map

	// exchange mints one identity token and trades it for Google credentials,
	// returning the credentials and discarding the machinery. A seam for
	// tests; production wiring is defaultExchange.
	exchange func(ctx context.Context, cfg oidcxgcp.Config, src plugin.OidcTokenSource) (*oauth2.Token, error)

	// now reads the clock. A seam for tests, which need to age an entry
	// without sleeping.
	now func() time.Time
}

// credEntry is one cached credential. No context, no token source, no method
// that could reach either.
type credEntry struct {
	accessToken string
	expiry      time.Time
}

// refreshMargin is how long before expiry a credential stops being handed out.
//
// It exists because the credential is handed to a Google client that will use
// it for the rest of this plugin call without asking again: a token that is
// valid now but expires mid-call would fail the request rather than trigger a
// refresh. The margin is generous relative to a single call, which is one page
// of a list or one CRUD operation.
const refreshMargin = 5 * time.Minute

// NewOidcDeps builds the OidcDeps a Plugin instance owns, wired to exchange
// identity tokens for Google credentials for real.
func NewOidcDeps(src plugin.OidcTokenSource) *OidcDeps {
	return &OidcDeps{Source: src, exchange: defaultExchange, now: time.Now}
}

// defaultExchange builds oidcx's external-account source, takes exactly one
// token from it, and throws the source away.
//
// Construct-and-discard is the mechanism that keeps the mint inside the
// caller's context. oidcx hands that context to Google's externalaccount
// package, which stores it and replays it into every later refresh - so a
// source kept beyond this call would keep minting under a context whose actor
// is gone. Taking one token while the caller is still on the stack means the
// only mint happens while their operator is Running. The AWS plugin does the
// same thing for the same reason, rebuilding its web-identity provider inside
// every Retrieve.
func defaultExchange(ctx context.Context, cfg oidcxgcp.Config, src plugin.OidcTokenSource) (*oauth2.Token, error) {
	ts, err := oidcxgcp.TokenSource(ctx, brokerClient{src: src}, cfg)
	if err != nil {
		return nil, err
	}
	return ts.Token()
}

// brokerClient adapts the plugin SDK's token source to the oidcx.Client the
// exchange expects. The audience oidcx asks for is the provider resource name,
// which is exactly what the broker's allowlist validates.
type brokerClient struct{ src plugin.OidcTokenSource }

func (b brokerClient) Token(ctx context.Context, audience string) (string, error) {
	return b.src.IdentityToken(ctx, audience)
}

// oidcAuth is the Oidc variant of the target config's auth block.
type oidcAuth struct {
	WorkloadIdentityProvider string `json:"WorkloadIdentityProvider"`
}

// credentialsFor resolves one Oidc auth block into Google credentials, using
// the cache when it holds something comfortably valid and exchanging under the
// caller's own context when it does not.
//
// The provider name is parsed with the same package the provisioner and the
// broker use. Parsing it here rather than splitting the string by hand is the
// point: the name is also the token audience, and a spelling that differs from
// the provisioned one produces a token that fails to exchange with an error
// that reads like an unrelated auth problem.
func (d *OidcDeps) credentialsFor(ctx context.Context, raw []byte, scopes []string) (*oauth2.Token, error) {
	var auth oidcAuth
	if err := json.Unmarshal(raw, &auth); err != nil {
		return nil, fmt.Errorf("config: malformed Oidc auth block: %w", err)
	}
	if auth.WorkloadIdentityProvider == "" {
		return nil, errors.New("config: Oidc auth requires WorkloadIdentityProvider")
	}

	name, err := gcpname.Parse(auth.WorkloadIdentityProvider)
	if err != nil {
		return nil, fmt.Errorf("config: Oidc auth WorkloadIdentityProvider is not a workload identity provider resource name: %w", err)
	}

	key := name.String() + "\n" + strings.Join(sortedCopy(scopes), " ")
	if cached, ok := d.creds.Load(key); ok {
		entry := cached.(credEntry)
		if entry.expiry.After(d.clock().Add(refreshMargin)) {
			return &oauth2.Token{AccessToken: entry.accessToken, Expiry: entry.expiry}, nil
		}
	}

	cfg := oidcxgcp.NewConfig(scopes)
	cfg.ProjectNumber = name.ProjectNumber
	cfg.PoolID = name.Pool
	cfg.ProviderID = name.Provider

	tok, err := d.exchangeFn()(ctx, cfg, d.Source)
	if err != nil {
		return nil, err
	}

	// Store, not LoadOrStore: this call exchanged because what was cached was
	// spent, so the fresh credential has to replace it rather than lose to it.
	//
	// Two operators refreshing the same key at once each mint, and the later
	// write wins. That is a duplicated broker call, not a correctness problem,
	// and the alternative - holding a lock across a cross-process call - would
	// block one actor's goroutine on another's.
	d.creds.Store(key, credEntry{accessToken: tok.AccessToken, expiry: tok.Expiry})
	return tok, nil
}

// exchangeFn returns the configured exchange, defaulting when a zero-value
// OidcDeps was built by hand rather than through NewOidcDeps.
func (d *OidcDeps) exchangeFn() func(context.Context, oidcxgcp.Config, plugin.OidcTokenSource) (*oauth2.Token, error) {
	if d.exchange != nil {
		return d.exchange
	}
	return defaultExchange
}

// clock reads the configured clock, defaulting as exchangeFn does.
func (d *OidcDeps) clock() time.Time {
	if d.now != nil {
		return d.now()
	}
	return time.Now()
}

// oidcClientOptions resolves an Oidc auth block into the Google client options
// that authenticate with it.
func (c *Config) oidcClientOptions(ctx context.Context, raw []byte, scopes []string) ([]option.ClientOption, error) {
	if c.deps == nil || c.deps.Source == nil {
		return nil, errors.New("config: Oidc auth requires an OIDC token source, but this plugin instance has none wired " +
			"(failing closed rather than falling back to ambient credentials)")
	}
	tok, err := c.deps.credentialsFor(ctx, raw, scopes)
	if err != nil {
		return nil, err
	}
	// A static source: the credential was resolved above, under this call's
	// context, and stays fixed for the rest of the call. Handing the client a
	// source that could refresh itself is exactly what must not happen here -
	// it would refresh whenever Google decided, on Google's goroutine.
	return []option.ClientOption{option.WithTokenSource(oauth2.StaticTokenSource(tok))}, nil
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
