// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package config

import (
	"context"
	"encoding/json"
	"os"

	"cloud.google.com/go/auth/credentials"
	"google.golang.org/api/option"
	pkgmodel "github.com/platform-engineering-labs/formae/pkg/model"
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
}

// ToClientOptions converts the config to Google API client options.
// Credentials are read from environment variables (GCP_CREDENTIALS_JSON or GCP_CREDENTIALS_FILE)
// to avoid storing sensitive data in the target config/database.
func (c *Config) ToClientOptions(_ context.Context) ([]option.ClientOption, error) {
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

// FromTargetConfig converts a target config JSON to GCP config
func FromTargetConfig(targetConfig json.RawMessage) *Config {
	if targetConfig == nil {
		return &Config{}
	}
	config := &Config{}
	_ = json.Unmarshal(targetConfig, config)
	return config
}

// FromTarget converts a Formae target to GCP config
func FromTarget(target *pkgmodel.Target) *Config {
	if target == nil || target.Config == nil {
		return &Config{}
	}
	return FromTargetConfig(target.Config)
}
