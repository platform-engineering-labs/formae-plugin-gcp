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

	// sources holds one oauth2.TokenSource per distinct (provider, scopes)
	// pair, built lazily. Google's external-account token source does its own
	// caching and refresh, so this map exists to avoid rebuilding the
	// exchange machinery per operation, not to cache tokens.
	sources sync.Map

	// newTokenSource builds the underlying exchange. A seam for tests, which
	// count constructions; production wiring is oidcx/gcp.TokenSource.
	newTokenSource func(ctx context.Context, client oidcxgcp.Config, source plugin.OidcTokenSource) (oauth2.TokenSource, error)
}

// NewOidcDeps builds the OidcDeps a Plugin instance owns, wired to exchange
// identity tokens for Google credentials for real.
func NewOidcDeps(src plugin.OidcTokenSource) *OidcDeps {
	return &OidcDeps{Source: src, newTokenSource: defaultTokenSource}
}

func defaultTokenSource(ctx context.Context, cfg oidcxgcp.Config, src plugin.OidcTokenSource) (oauth2.TokenSource, error) {
	return oidcxgcp.TokenSource(ctx, brokerClient{src: src}, cfg)
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

// tokenSourceFor resolves the token source for one Oidc auth block, building
// it once per (provider, scopes) pair.
//
// The provider name is parsed with the same package the provisioner and the
// broker use. Parsing it here rather than splitting the string by hand is the
// point: the name is also the token audience, and a spelling that differs from
// the provisioned one produces a token that fails to exchange with an error
// that reads like an unrelated auth problem.
func (d *OidcDeps) tokenSourceFor(ctx context.Context, raw []byte, scopes []string) (oauth2.TokenSource, error) {
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
	if existing, ok := d.sources.Load(key); ok {
		return existing.(oauth2.TokenSource), nil
	}

	cfg := oidcxgcp.NewConfig(scopes)
	cfg.ProjectNumber = name.ProjectNumber
	cfg.PoolID = name.Pool
	cfg.ProviderID = name.Provider

	build := d.newTokenSource
	if build == nil {
		build = defaultTokenSource
	}
	ts, err := build(ctx, cfg, d.Source)
	if err != nil {
		return nil, err
	}

	// LoadOrStore, not Store: two operations racing here must end up sharing
	// one source rather than each keeping its own.
	actual, _ := d.sources.LoadOrStore(key, ts)
	return actual.(oauth2.TokenSource), nil
}

// oidcClientOptions resolves an Oidc auth block into the Google client options
// that authenticate with it.
func (c *Config) oidcClientOptions(ctx context.Context, raw []byte, scopes []string) ([]option.ClientOption, error) {
	if c.deps == nil || c.deps.Source == nil {
		return nil, errors.New("config: Oidc auth requires an OIDC token source, but this plugin instance has none wired " +
			"(failing closed rather than falling back to ambient credentials)")
	}
	ts, err := c.deps.tokenSourceFor(ctx, raw, scopes)
	if err != nil {
		return nil, err
	}
	return []option.ClientOption{option.WithTokenSource(ts)}, nil
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
