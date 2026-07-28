// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package logging

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// LoggingAPI configuration for the Cloud Logging API v2.
var LoggingAPI = base.APIConfig{
	BaseURL:     "https://logging.googleapis.com/v2",
	APIVersion:  "v2",
	PathBuilder: loggingPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// LoggingOperations - Cloud Logging log-metric operations are synchronous:
// metrics.create/update return the LogMetric directly and metrics.delete
// returns google.protobuf.Empty. No long-running Operation is involved.
var LoggingOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractLoggingNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// LoggingNativeID - full resource path "projects/{project}/metrics/{name}".
var LoggingNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseLoggingNativeID,
}

// loggingPathBuilder builds /projects/{project}/{resourceType}[/{name}].
// Log-based metrics are project-scoped (projects/{project}/metrics/{id}).
func loggingPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/%s", ctx.Project, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractLoggingNativeID builds the full resource path. Logging responses carry
// the short metric id in "name"; the path is project + resourceType + name.
func extractLoggingNativeID(response map[string]interface{}, ctx base.PathContext) string {
	name := ctx.ResourceName
	if name == "" {
		if n, ok := response["name"].(string); ok {
			name = n
		}
	}
	if name == "" {
		return ""
	}
	return fmt.Sprintf("projects/%s/%s/%s", ctx.Project, ctx.ResourceType, name)
}

// parseLoggingNativeID parses "projects/{project}/{resourceType}/{name}".
func parseLoggingNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 4 || parts[0] != "projects" {
		return base.PathContext{}, fmt.Errorf("invalid Logging native ID: %s", nativeID)
	}
	return base.PathContext{
		Project:      parts[1],
		ResourceType: parts[2],
		ResourceName: parts[3],
	}, nil
}
