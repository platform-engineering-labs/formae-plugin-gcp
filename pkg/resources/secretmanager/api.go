// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package secretmanager

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// SecretManagerAPI configuration for the Secret Manager API v1.
var SecretManagerAPI = base.APIConfig{
	BaseURL:     "https://secretmanager.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: secretManagerPathBuilder,
	// Secret Manager uses pageSize and rejects "maxResults" with 400.
	Pagination: &base.PaginationConfig{PageSizeParam: "pageSize", PageTokenParam: "pageToken"},
}

// SecretManagerOperations - Secret Manager admin operations are synchronous.
var SecretManagerOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractSecretManagerNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// SecretManagerNativeID - full resource path "projects/{project}/secrets/{secret}".
var SecretManagerNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseSecretManagerNativeID,
}

// secretManagerPathBuilder builds Secret Manager paths:
//   - collection: /projects/{project}/secrets
//   - resource:   /projects/{project}/secrets/{name}
func secretManagerPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/%s", ctx.Project, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractSecretManagerNativeID returns the full resource path. Responses carry
// it in the "name" field as "projects/{p}/secrets/{name}".
func extractSecretManagerNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name, ok := response["name"].(string); ok && strings.HasPrefix(name, "projects/") {
		return name
	}
	if ctx.ResourceName != "" {
		return fmt.Sprintf("projects/%s/%s/%s", ctx.Project, ctx.ResourceType, ctx.ResourceName)
	}
	return ""
}

// parseSecretManagerNativeID parses "projects/{project}/secrets/{name}".
func parseSecretManagerNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 4 || parts[0] != "projects" {
		return base.PathContext{}, fmt.Errorf("invalid secret manager native ID: %s", nativeID)
	}
	return base.PathContext{
		Project:      parts[1],
		ResourceType: parts[2],
		ResourceName: parts[3],
	}, nil
}
