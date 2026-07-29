// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package monitoring

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// MonitoringAPI - Cloud Monitoring API v3. Resources are project-scoped and
// create/get/patch/delete are synchronous (the mutated resource is returned
// directly; there is no long-running operation).
var MonitoringAPI = base.APIConfig{
	BaseURL:     "https://monitoring.googleapis.com/v3",
	APIVersion:  "v3",
	PathBuilder: monitoringPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

var MonitoringOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractMonitoringNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// MonitoringNativeID - full path "projects/{project}/{resourceType}/{name}".
// The default FullPathFormat parser handles this shape.
var MonitoringNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
}

// monitoringPathBuilder builds /projects/{project}/{resourceType}[/{name}].
func monitoringPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/%s", ctx.Project, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractMonitoringNativeID returns the full resource path. Monitoring echoes
// the fully-qualified name in "name" (e.g. "projects/p/notificationChannels/123");
// fall back to building it from context when absent.
func extractMonitoringNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name, ok := response["name"].(string); ok && name != "" {
		if i := strings.Index(name, "projects/"); i >= 0 {
			return name[i:]
		}
	}
	if ctx.ResourceName != "" {
		return fmt.Sprintf("projects/%s/%s/%s", ctx.Project, ctx.ResourceType, ctx.ResourceName)
	}
	return ""
}
