package testutil

import (
	"encoding/json"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
)

var (
	Project       = "development-477117"
	ProjectNumber = "989754770009" // Project number for convenience groups
	Region        = "europe-central2"
	Zone          = "europe-central2-b"

	Config = &config.Config{
		Project:         Project,
		Region:          Region,
		CredentialsFile: "/Users/stheno/git/pel/gcp.nico.json",
	}

	// TargetConfig is a json.RawMessage containing the target configuration
	TargetConfig = func() json.RawMessage {
		b, _ := json.Marshal(map[string]interface{}{
			"Project":         Project,
			"Region":          Region,
			"CredentialsFile": Config.CredentialsFile,
		})
		return b
	}()
)
