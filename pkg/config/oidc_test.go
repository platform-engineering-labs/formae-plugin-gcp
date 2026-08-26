// © 2026 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package config

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/oauth2"

	"github.com/platform-engineering-labs/formae/pkg/plugin"
	oidcxgcp "github.com/platform-engineering-labs/oox/oidcx/gcp"
)

// goldenProvider is a canonical provider resource name. oox/gcpname pins the
// same literal, and so does the provisioner that creates it.
const goldenProvider = "//iam.googleapis.com/projects/123456789012/locations/global/workloadIdentityPools/formae-ai/providers/formae-ai"

func oidcTargetConfig(t *testing.T, provider string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"Project": "test-project",
		"Auth": map[string]any{
			"Type":                     "Oidc",
			"WorkloadIdentityProvider": provider,
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

type fakeTokenSource struct{ calls atomic.Int64 }

func (f *fakeTokenSource) IdentityToken(_ context.Context, _ string) (string, error) {
	f.calls.Add(1)
	return "an-identity-token", nil
}

// countingDeps builds deps whose token-source construction is counted, so a
// cache test can assert one construction rather than the equality of two
// opaque option values.
func countingDeps(src plugin.OidcTokenSource, built *atomic.Int64, seen *[]oidcxgcp.Config) *OidcDeps {
	d := NewOidcDeps(src)
	d.newTokenSource = func(_ context.Context, cfg oidcxgcp.Config, _ plugin.OidcTokenSource) (oauth2.TokenSource, error) {
		built.Add(1)
		if seen != nil {
			*seen = append(*seen, cfg)
		}
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "x"}), nil
	}
	return d
}

// TestOidcAuthWithoutBrokerFailsClosed is the property that matters most: an
// agent with no paired broker must refuse an Oidc target, not quietly
// authenticate as itself with whatever credentials the environment happens to
// hold.
func TestOidcAuthWithoutBrokerFailsClosed(t *testing.T) {
	cfg := FromTargetConfig(oidcTargetConfig(t, goldenProvider)) // no WithOidcDeps

	opts, err := cfg.ToClientOptions(t.Context())
	if err == nil {
		t.Fatalf("ToClientOptions succeeded with no token source; returned %d options", len(opts))
	}
	if opts != nil {
		t.Error("options were returned alongside the error")
	}
	if !strings.Contains(err.Error(), "failing closed") {
		t.Errorf("error does not say it is failing closed: %v", err)
	}
}

// TestNilSourceAlsoFailsClosed covers deps that exist but carry no source,
// which is what an agent too old to pair a broker produces.
func TestNilSourceAlsoFailsClosed(t *testing.T) {
	cfg := FromTargetConfig(oidcTargetConfig(t, goldenProvider)).WithOidcDeps(&OidcDeps{})

	if _, err := cfg.ToClientOptions(t.Context()); err == nil {
		t.Fatal("ToClientOptions succeeded with deps carrying no source")
	}
}

func TestAbsentAuthKeepsExistingBehaviour(t *testing.T) {
	// A config with no Auth block must not go near the OIDC path, even when
	// deps are wired: every existing target has no Auth block.
	var built atomic.Int64
	cfg := FromTargetConfig(json.RawMessage(`{"Project":"test-project"}`)).
		WithOidcDeps(countingDeps(&fakeTokenSource{}, &built, nil))

	// Credential detection may fail in a bare test environment; what matters
	// is that the OIDC exchange was not built.
	_, _ = cfg.ToClientOptions(t.Context())

	if built.Load() != 0 {
		t.Errorf("an absent Auth block built an OIDC token source %d times", built.Load())
	}
}

func TestUnknownAuthTypeIsRejected(t *testing.T) {
	raw := json.RawMessage(`{"Project":"p","Auth":{"Type":"SomethingElse"}}`)
	cfg := FromTargetConfig(raw).WithOidcDeps(NewOidcDeps(&fakeTokenSource{}))

	_, err := cfg.ToClientOptions(t.Context())
	if err == nil {
		t.Fatal("an unknown Auth type was accepted")
	}
	if !strings.Contains(err.Error(), "unknown Auth type") {
		t.Errorf("error = %v, want it to name the unknown type", err)
	}
}

func TestAuthBlockWithoutTypeIsRejected(t *testing.T) {
	raw := json.RawMessage(`{"Project":"p","Auth":{"WorkloadIdentityProvider":"` + goldenProvider + `"}}`)
	cfg := FromTargetConfig(raw).WithOidcDeps(NewOidcDeps(&fakeTokenSource{}))

	if _, err := cfg.ToClientOptions(t.Context()); err == nil {
		t.Fatal("an Auth block with no discriminator was accepted")
	}
}

// TestMalformedProviderNameFailsBeforeAnyAPICall pins where validation
// happens. ToClientOptions is the single credential seam, so a bad name must
// fail here rather than surfacing much later as a token that will not
// exchange.
func TestMalformedProviderNameFailsBeforeAnyAPICall(t *testing.T) {
	var built atomic.Int64
	bad := []string{
		"",
		"not-a-resource-name",
		"//iam.googleapis.com.evil/projects/123456789012/locations/global/workloadIdentityPools/formae-ai/providers/formae-ai",
		"https://iam.googleapis.com/projects/123456789012/locations/global/workloadIdentityPools/formae-ai/providers/formae-ai",
	}
	for _, provider := range bad {
		cfg := FromTargetConfig(oidcTargetConfig(t, provider)).
			WithOidcDeps(countingDeps(&fakeTokenSource{}, &built, nil))

		if _, err := cfg.ToClientOptions(t.Context()); err == nil {
			t.Errorf("provider %q was accepted", provider)
		}
	}
	if built.Load() != 0 {
		t.Errorf("a malformed provider name still built a token source %d times", built.Load())
	}
}

func TestTokenSourceConstructedOncePerProviderAndScopes(t *testing.T) {
	var built atomic.Int64
	var seen []oidcxgcp.Config
	deps := countingDeps(&fakeTokenSource{}, &built, &seen)

	same := func() *Config {
		return FromTargetConfig(oidcTargetConfig(t, goldenProvider)).WithOidcDeps(deps)
	}
	if _, err := same().ToClientOptions(t.Context()); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := same().ToClientOptions(t.Context()); err != nil {
		t.Fatalf("second: %v", err)
	}
	if built.Load() != 1 {
		t.Errorf("token source built %d times for one provider, want 1", built.Load())
	}

	// Different scopes are a different credential and must not share.
	scoped := json.RawMessage(`{"Project":"p","Scopes":["https://www.googleapis.com/auth/devstorage.read_only"],` +
		`"Auth":{"Type":"Oidc","WorkloadIdentityProvider":"` + goldenProvider + `"}}`)
	if _, err := FromTargetConfig(scoped).WithOidcDeps(deps).ToClientOptions(t.Context()); err != nil {
		t.Fatalf("scoped: %v", err)
	}
	if built.Load() != 2 {
		t.Errorf("token source built %d times across two scope sets, want 2", built.Load())
	}
}

// TestProviderNameIsDecomposedForTheExchange checks the parts actually handed
// to the exchange, not just that parsing succeeded: a parser that returned the
// right error for bad input but the wrong components for good input would pass
// every other test here.
func TestProviderNameIsDecomposedForTheExchange(t *testing.T) {
	var built atomic.Int64
	var seen []oidcxgcp.Config
	deps := countingDeps(&fakeTokenSource{}, &built, &seen)

	cfg := FromTargetConfig(oidcTargetConfig(t, goldenProvider)).WithOidcDeps(deps)
	if _, err := cfg.ToClientOptions(t.Context()); err != nil {
		t.Fatalf("ToClientOptions: %v", err)
	}
	if len(seen) != 1 {
		t.Fatalf("saw %d exchange configs, want 1", len(seen))
	}
	got := seen[0]
	if got.ProjectNumber != "123456789012" || got.PoolID != "formae-ai" || got.ProviderID != "formae-ai" {
		t.Errorf("exchange config = %+v, want project 123456789012 / pool formae-ai / provider formae-ai", got)
	}
	// The audience the exchange requests is the provider name, which is what
	// the broker's allowlist validates and what the provider itself pins.
	if got.Audience() != goldenProvider {
		t.Errorf("audience = %q, want %q", got.Audience(), goldenProvider)
	}
}
