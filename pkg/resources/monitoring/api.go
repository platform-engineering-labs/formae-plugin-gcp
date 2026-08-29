// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package monitoring

import (
	"fmt"
	"net/url"
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
	if ctx.IsList && ctx.ResourceType == "metricDescriptors" {
		path += "?filter=" + url.QueryEscape(userOwnedMetricFilter)
	}
	return path
}

// userOwnedMetricFilter restricts a metricDescriptors list to the descriptors a
// project actually owns.
//
// metricDescriptors.list otherwise returns every descriptor visible to the
// project - well over a thousand built-in ones for GCP's own services - and a
// custom metric is simply somewhere in that pile, quite possibly not on the
// first page. Discovery listed, never saw the descriptor it had just created,
// and timed out.
//
// Filtering is not merely an optimisation here: a built-in descriptor cannot be
// created, changed or deleted, so it is not a resource formae can manage and
// has no business appearing in discovery.
//
// It is one prefix and not two because the API rejects the obvious form:
// "metric.type = starts_with(...) OR metric.type = starts_with(...)" answers
// HTTP 400, "Within the 'metric' prefix, OR can only be used to connect a list
// of 'labels'". A rejected filter fails the whole list, which reads downstream
// as "the resource is gone" and had sync tombstone a descriptor that existed.
// external.googleapis.com/user/ descriptors are therefore not listed; they are
// written by the Cloud Monitoring agent rather than by a forma.
const userOwnedMetricFilter = `metric.type = starts_with("custom.googleapis.com/")`

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

// normalizeMonitoringProject rewrites the project segment of a Monitoring
// resource path to the project the caller configured.
//
// Monitoring answers with whichever form it likes: dashboards.create returns
// "projects/{project_number}/dashboards/x" while dashboards.list returns
// "projects/{project_id}/dashboards/x" for that same dashboard. A native ID
// taken verbatim therefore depends on which call produced it, so a dashboard
// created as projects/1234567890/... is rediscovered as projects/my-project/...
// and the two never correlate - the managed resource appears a second time as
// an unmanaged one.
func normalizeMonitoringProject(path, project string) string {
	if project == "" {
		return path
	}
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] != "projects" {
		return path
	}
	parts[1] = project
	return strings.Join(parts, "/")
}

// extractMonitoringNativeID returns the full resource path. Monitoring echoes
// the fully-qualified name in "name" (e.g. "projects/p/notificationChannels/123");
// fall back to building it from context when absent.
func extractMonitoringNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name, ok := response["name"].(string); ok && name != "" {
		if i := strings.Index(name, "projects/"); i >= 0 {
			return normalizeMonitoringProject(name[i:], ctx.Project)
		}
	}
	if ctx.ResourceName != "" {
		return fmt.Sprintf("projects/%s/%s/%s", ctx.Project, ctx.ResourceType, ctx.ResourceName)
	}
	return ""
}
