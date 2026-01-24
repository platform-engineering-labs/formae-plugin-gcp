// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package config

import (
	"context"
	"encoding/json"

	"google.golang.org/api/option"
	pkgmodel "github.com/platform-engineering-labs/formae/pkg/model"
)

// Config represents GCP configuration for a target
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

	// CredentialsFile is the path to the service account JSON key file
	CredentialsFile string `json:"CredentialsFile,omitempty"`

	// CredentialsJSON is the inline service account JSON key
	CredentialsJSON string `json:"CredentialsJSON,omitempty"`

	// Scopes are the OAuth2 scopes to request
	Scopes []string `json:"Scopes,omitempty"`
}

// ToClientOptions converts the config to Google API client options
func (c *Config) ToClientOptions(ctx context.Context) ([]option.ClientOption, error) {
	var opts []option.ClientOption

	// Handle credentials
	if c.CredentialsJSON != "" {
		// Use inline JSON credentials
		opts = append(opts, option.WithCredentialsJSON([]byte(c.CredentialsJSON)))
	} else if c.CredentialsFile != "" {
		// Use credentials file
		opts = append(opts, option.WithCredentialsFile(c.CredentialsFile))
	}
	// If neither is specified, Application Default Credentials (ADC) will be used

	// Add custom scopes if specified
	if len(c.Scopes) > 0 {
		opts = append(opts, option.WithScopes(c.Scopes...))
	} else {
		// Default scopes for compute, storage, and IAM
		opts = append(opts, option.WithScopes(
			"https://www.googleapis.com/auth/cloud-platform",
			"https://www.googleapis.com/auth/compute",
			"https://www.googleapis.com/auth/devstorage.full_control",
		))
	}

	return opts, nil
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
