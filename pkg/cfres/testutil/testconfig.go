package testutil

import (
	"encoding/json"
	"os"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
)

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

var (
	Project       = getEnvOrDefault("GCP_PROJECT_ID", "development-477117")
	ProjectNumber = getEnvOrDefault("GCP_PROJECT_NUMBER", "989754770009")
	Region        = getEnvOrDefault("GCP_REGION", "europe-central2")
	Zone          = getEnvOrDefault("GCP_ZONE", "europe-central2-b")

	// CredentialsFile from GCP_CREDENTIALS_FILE env var.
	// If empty, Application Default Credentials (ADC) will be used.
	CredentialsFile = os.Getenv("GCP_CREDENTIALS_FILE")

	Config = &config.Config{
		Project:         Project,
		Region:          Region,
		CredentialsFile: CredentialsFile,
	}

	// TargetConfig is a json.RawMessage containing the target configuration
	TargetConfig = func() json.RawMessage {
		cfg := map[string]interface{}{
			"Project": Project,
			"Region":  Region,
		}
		if CredentialsFile != "" {
			cfg["CredentialsFile"] = CredentialsFile
		}
		b, _ := json.Marshal(cfg)
		return b
	}()
)
