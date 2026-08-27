// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"cloud.google.com/go/auth/credentials"
	pkgmodel "github.com/platform-engineering-labs/formae/pkg/model"
	"google.golang.org/api/option"
)

// Config represents GCP configuration for a target.
// Note: Credentials are NOT stored in the target config to avoid persisting
// sensitive data in the database. Instead, credentials are read from
// environment variables (GCP_CREDENTIALS_JSON or GCP_CREDENTIALS_FILE).
type Config struct {
	// Project is the GCP project ID
	Project string `json:"Project"`

	// Region is the GCP region (e.g., "us-central1", "us-east1")
	Region string `json:"Region,omitempty"`

	// Zone is the GCP zone (e.g., "us-central1-a")
	Zone string `json:"Zone,omitempty"`

	// Location is used by Container (GKE) and CloudRun APIs
	// Can be a region or zone. For GKE, use "-" to target all locations.
	Location string `json:"Location,omitempty"`

	// Scopes are the OAuth2 scopes to request
	Scopes []string `json:"Scopes,omitempty"`

	// Auth is the target's authentication strategy, as rendered by the Pkl
	// schema. Absent means the historical behaviour: environment variables,
	// then Application Default Credentials.
	Auth json.RawMessage `json:"Auth,omitempty"`

	// deps carries what the plugin instance owns: the OIDC token source and
	// the per-instance token-source cache. Never serialized; nil deps means
	// Oidc auth fails closed.
	deps *OidcDeps
}

// Deps returns the OIDC deps this config carries, so a config derived from
// another can inherit them.
//
// Deriving is where they used to get lost: a helper that copied Project and
// Region off a parent config produced something that looked complete and
// failed closed on the first Oidc target it met.
func (c *Config) Deps() *OidcDeps {
	if c == nil {
		return nil
	}
	return c.deps
}

// WithOidcDeps threads the plugin instance's OidcDeps onto an existing Config.
//
// Kept for the paths that already hold a Config. New code should pass deps to
// FromTargetConfig instead, which cannot produce an unwired one.
func (c *Config) WithOidcDeps(d *OidcDeps) *Config {
	c.deps = d
	return c
}

// authDiscriminator is the shape every Auth block variant shares.
type authDiscriminator struct {
	Type string `json:"Type"`
}

// effectiveAuth reports the auth type this config resolves to. An absent
// block means the default chain, which is what every target used before the
// block existed.
//
// An unknown Type is an error rather than a fall-through: silently treating
// an auth block nobody understands as "use ambient credentials" is how a
// hosted agent ends up acting as itself instead of as the customer.
func (c *Config) effectiveAuth() (string, []byte, error) {
	if len(c.Auth) == 0 || string(c.Auth) == "null" {
		return "", nil, nil
	}
	var disc authDiscriminator
	if err := json.Unmarshal(c.Auth, &disc); err != nil {
		return "", nil, fmt.Errorf("config: malformed Auth block: %w", err)
	}
	if disc.Type == "" {
		return "", nil, errors.New("config: Auth block is missing its Type discriminator")
	}
	if disc.Type != AuthTypeOidc {
		return "", nil, fmt.Errorf("config: unknown Auth type %q", disc.Type)
	}
	return disc.Type, c.Auth, nil
}

// ToClientOptions converts the config to Google API client options.
//
// This is the single credential seam, so it is also where an auth block is
// validated: a malformed provider name fails here, before any Google client
// is constructed and any API call is made.
//
// With no auth block, credentials are read from environment variables
// (GCP_CREDENTIALS_JSON or GCP_CREDENTIALS_FILE) or Application Default
// Credentials, exactly as before.
func (c *Config) ToClientOptions(ctx context.Context) ([]option.ClientOption, error) {
	// Determine scopes so credentials are minted with them.
	scopes := c.Scopes
	if len(scopes) == 0 {
		// Default scopes for compute, storage, and IAM
		scopes = []string{
			"https://www.googleapis.com/auth/cloud-platform",
			"https://www.googleapis.com/auth/compute",
			"https://www.googleapis.com/auth/devstorage.full_control",
		}
	}

	authType, rawAuth, err := c.effectiveAuth()
	if err != nil {
		return nil, err
	}
	if authType == AuthTypeOidc {
		return c.oidcClientOptions(ctx, rawAuth, scopes)
	}

	// Resolve credentials from environment variables, falling back to ADC.
	// Priority: GCP_CREDENTIALS_JSON > GCP_CREDENTIALS_FILE > ADC.
	//
	// The CredentialsJSON/CredentialsFile overrides are marked deprecated
	// (SA1019) because accepting a credential config from an *untrusted* source
	// is risky. Here the source is the operator's own environment variables —
	// the documented, trusted interface of this plugin — so the override is
	// intentional.
	detectOpts := &credentials.DetectOptions{Scopes: scopes}
	if credJSON := os.Getenv("GCP_CREDENTIALS_JSON"); credJSON != "" {
		detectOpts.CredentialsJSON = []byte(credJSON) //nolint:staticcheck // trusted operator-provided credentials via env var
	} else if credFile := os.Getenv("GCP_CREDENTIALS_FILE"); credFile != "" {
		detectOpts.CredentialsFile = credFile //nolint:staticcheck // trusted operator-provided credentials via env var
	}

	creds, err := credentials.DetectDefault(detectOpts)
	if err != nil {
		return nil, err
	}

	return []option.ClientOption{option.WithAuthCredentials(creds)}, nil
}

// FromTargetConfig converts a target config JSON to GCP config.
//
// deps is required rather than threaded afterwards. It was optional once, and
// of the call sites that produced a Config exactly one remembered to attach
// deps: every other one built a config that failed closed the moment it met an
// Oidc target. Taking deps here makes that omission a compile error instead of
// a credential failure discovered in a customer's project. Pass nil only where
// no Oidc target can reach the config, and say why at the call site.
func FromTargetConfig(targetConfig json.RawMessage, deps *OidcDeps) *Config {
	if targetConfig == nil {
		return &Config{deps: deps}
	}
	config := &Config{}
	_ = json.Unmarshal(targetConfig, config)
	config.deps = deps
	return config
}

// FromTarget converts a Formae target to GCP config
func FromTarget(target *pkgmodel.Target, deps *OidcDeps) *Config {
	if target == nil || target.Config == nil {
		return &Config{deps: deps}
	}
	return FromTargetConfig(target.Config, deps)
}
