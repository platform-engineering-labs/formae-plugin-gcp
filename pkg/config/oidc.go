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

	d.cache(key, tok)
	return tok, nil
}

// cache keeps the credential that stays usable longest.
//
// Concurrent operators refreshing one key each mint, which is a duplicated
// broker call rather than a correctness problem: holding a lock across a
// cross-process call would block one actor's goroutine on another's. But
// "last write wins" orders by when an exchange finished, not by how long its
// result lives, so a slow exchange could land after a fast one and replace a
// fresher credential with a staler one. Compare instead, and retry rather than
// clobber a concurrent writer.
//
// A credential with no expiry is never cached. Google's STS always returns one,
// so this is defensive: an entry that cannot be reasoned about would either be
// treated as permanently spent (re-exchanged by every operation) or trusted
// forever, and neither is a cache.
func (d *OidcDeps) cache(key string, tok *oauth2.Token) {
	if tok.Expiry.IsZero() {
		return
	}
	entry := credEntry{accessToken: tok.AccessToken, expiry: tok.Expiry}
	for {
		prev, loaded := d.creds.LoadOrStore(key, entry)
		if !loaded {
			return
		}
		if !entry.expiry.After(prev.(credEntry).expiry) {
			return
		}
		if d.creds.CompareAndSwap(key, prev, entry) {
			return
		}
	}
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
	// Resolve once now so a broken auth block fails here rather than on the
	// first request, then hand the client a source that can refresh.
	if _, err := c.deps.credentialsFor(ctx, raw, scopes); err != nil {
		return nil, err
	}
	return []option.ClientOption{option.WithTokenSource(c.callSource(ctx, raw, scopes))}, nil
}

// callSource builds the token source for one plugin call. Separate from
// oidcClientOptions so a test can hold the same source the Google client is
// handed, rather than a lookalike built beside it.
func (c *Config) callSource(ctx context.Context, raw []byte, scopes []string) oauth2.TokenSource {
	return &callTokenSource{deps: c.deps, ctx: ctx, raw: raw, scopes: scopes}
}

// callTokenSource authenticates one plugin call, and must not outlive it.
//
// A static token cannot work here. Several list implementations build one
// client and then page through everything with it - secretmanager walks every
// secret and every version of each - so one call can outrun any margin we could
// put on a credential fixed up front. Google's client would keep presenting the
// expired token, because a static source has nothing else to return.
//
// So this refreshes, and holding ctx is what makes that safe rather than a
// repeat of the bug it replaces. The context belongs to the operation on the
// stack; the source is built in ToClientOptions and handed to a client built in
// the same function, both of which are discarded when the call returns. Every
// refresh therefore happens inside that call, on the operator's own goroutine,
// while the operator is Running - which is the only state Ergo will let the
// mint out in.
//
// The invariant is the lifetime: NEVER cache this, or anything holding it,
// beyond the call it was built for. Cache credentials instead - OidcDeps.creds
// holds those as data precisely so nothing durable needs a context.
type callTokenSource struct {
	deps   *OidcDeps
	ctx    context.Context
	raw    []byte
	scopes []string
}

func (s *callTokenSource) Token() (*oauth2.Token, error) {
	return s.deps.credentialsFor(s.ctx, s.raw, s.scopes)
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
