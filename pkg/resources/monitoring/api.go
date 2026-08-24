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

// MonitoringDashboardAPI - Cloud Monitoring dashboards live on v1, not v3, so
// they need their own base URL. Path shape and sync semantics are identical.
var MonitoringDashboardAPI = base.APIConfig{
	BaseURL:     "https://monitoring.googleapis.com/v1",
	APIVersion:  "v1",
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

// MonitoringMetricDescriptorNativeID - a metric descriptor's id is its metric
// type ("custom.googleapis.com/foo/bar"), which contains slashes, so the
// pairwise path parser cannot be used.
var MonitoringMetricDescriptorNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseMetricDescriptorNativeID,
}

const metricDescriptorsSegment = "/metricDescriptors/"

// parseMetricDescriptorNativeID parses
// "projects/{p}/metricDescriptors/{metric.type}" — everything after the
// collection segment is the type, slashes included.
func parseMetricDescriptorNativeID(nativeID string) (base.PathContext, error) {
	i := strings.Index(nativeID, metricDescriptorsSegment)
	if !strings.HasPrefix(nativeID, "projects/") || i < 0 {
		return base.PathContext{}, fmt.Errorf("invalid metric descriptor native ID: %s", nativeID)
	}
	project := nativeID[len("projects/"):i]
	metricType := nativeID[i+len(metricDescriptorsSegment):]
	if project == "" || metricType == "" {
		return base.PathContext{}, fmt.Errorf("invalid metric descriptor native ID: %s", nativeID)
	}
	return base.PathContext{
		Project:      project,
		ResourceType: "metricDescriptors",
		ResourceName: metricType,
	}, nil
}

// MonitoringNativeID - full path "projects/{project}/{resourceType}/{name}".
// The default FullPathFormat parser handles this shape.
var MonitoringNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
}

// monitoringPathBuilder builds /projects/{project}/{resourceType}[/{name}], or
// /projects/{project}/{parentType}/{parent}/{resourceType}[/{name}] for
// resources nested under another (SLOs under a service).
func monitoringPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s", ctx.Project)
	switch {
	case ctx.ParentType != "" && ctx.ParentResource != "":
		path += fmt.Sprintf("/%s/%s", ctx.ParentType, ctx.ParentResource)
	case ctx.ResourceType == "serviceLevelObjectives":
		// Discovery lists with no properties, so no owning service is known.
		// "services/-" lists the SLOs of every service, which is the only way an
		// SLO can be discovered rather than merely managed. Create and read
		// always carry a real service, so they never reach this branch.
		path += "/services/-"
	}
	path += "/" + ctx.ResourceType
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// MonitoringSloNativeID - SLOs are two levels deep, so the default pairwise
// path parser would lose the owning service. Parse it explicitly.
var MonitoringSloNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseSloNativeID,
}

// parseSloNativeID parses
// "projects/{project}/services/{service}/serviceLevelObjectives/{id}".
func parseSloNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 6 || parts[0] != "projects" || parts[2] != "services" ||
		parts[4] != "serviceLevelObjectives" {
		return base.PathContext{}, fmt.Errorf("invalid SLO native ID: %s", nativeID)
	}
	return base.PathContext{
		Project:        parts[1],
		ParentType:     "services",
		ParentResource: parts[3],
		ResourceType:   parts[4],
		ResourceName:   parts[5],
	}, nil
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
