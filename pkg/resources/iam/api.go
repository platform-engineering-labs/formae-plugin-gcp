// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package iam

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// IAMAPI configuration for the IAM Admin API v1 (service accounts, custom roles).
var IAMAPI = base.APIConfig{
	BaseURL:     "https://iam.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: iamPathBuilder,
	// IAM uses pageSize and rejects "maxResults" with 400.
	Pagination: &base.PaginationConfig{PageSizeParam: "pageSize", PageTokenParam: "pageToken"},
}

// IAMOperations - IAM admin operations are synchronous.
var IAMOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractIAMNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// IAMNativeID - full resource path, e.g.
// "projects/{project}/serviceAccounts/{email}" or "projects/{project}/roles/{id}".
var IAMNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseIAMNativeID,
}

// iamPathBuilder builds /projects/{project}/{resourceType}[/{name}].
func iamPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/%s", ctx.Project, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractIAMNativeID returns the full resource path from the "name" field.
func extractIAMNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name, ok := response["name"].(string); ok && strings.HasPrefix(name, "projects/") {
		return name
	}
	if ctx.ResourceName != "" {
		return fmt.Sprintf("projects/%s/%s/%s", ctx.Project, ctx.ResourceType, ctx.ResourceName)
	}
	return ""
}

// parseIAMNativeID parses "projects/{project}/{resourceType}/{name}".
func parseIAMNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 4 || parts[0] != "projects" {
		return base.PathContext{}, fmt.Errorf("invalid IAM native ID: %s", nativeID)
	}
	return base.PathContext{
		Project:      parts[1],
		ResourceType: parts[2],
		ResourceName: parts[3],
	}, nil
}
