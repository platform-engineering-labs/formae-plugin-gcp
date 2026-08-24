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

// LoggingLocationAPI - for resources that are location-scoped but have no
// parent resource (saved queries, log scopes): the shared builder yields
// /projects/{p}/locations/{loc}/{resourceType}.
var LoggingLocationAPI = base.APIConfig{
	BaseURL:     "https://logging.googleapis.com/v2",
	APIVersion:  "v2",
	PathBuilder: loggingLocationPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// LocationScopedNativeID returns a NativeIDConfig for
// "projects/{p}/locations/{loc}/{resourceType}/{id}". The default pairwise
// parser would drop the location, and each leaf type needs its own check so a
// native ID from a sibling type is rejected rather than silently mis-parsed.
func LocationScopedNativeID(resourceType string) base.NativeIDConfig {
	return base.NativeIDConfig{
		Format: base.FullPathFormat,
		Parser: func(nativeID string) (base.PathContext, error) {
			parts := strings.Split(nativeID, "/")
			if len(parts) != 6 || parts[0] != "projects" || parts[2] != "locations" ||
				parts[4] != resourceType {
				return base.PathContext{}, fmt.Errorf(
					"invalid Logging %s native ID: %s", resourceType, nativeID)
			}
			return base.PathContext{
				Project:      parts[1],
				Location:     parts[3],
				ResourceType: parts[4],
				ResourceName: parts[5],
			}, nil
		},
	}
}

// LoggingLocationOperations - sync, with the native ID taken from the
// response's fully-qualified name so the location survives the round-trip.
var LoggingLocationOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractLoggingLocationNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// extractLoggingLocationNativeID prefers the response's full resource name and
// falls back to composing the path from context (delete returns an empty body).
func extractLoggingLocationNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name, ok := response["name"].(string); ok {
		if i := strings.Index(name, "projects/"); i >= 0 {
			return name[i:]
		}
	}
	if ctx.ResourceName == "" {
		return ""
	}
	location := ctx.Location
	if location == "" {
		location = "global"
	}
	return fmt.Sprintf("projects/%s/locations/%s/%s/%s",
		ctx.Project, location, ctx.ResourceType, ctx.ResourceName)
}

// LoggingViewAPI - log views sit three levels deep
// (/projects/{p}/locations/{loc}/buckets/{bucket}/views/{id}), a shape the flat
// loggingPathBuilder cannot express. It gets its own builder rather than
// teaching the shared one about locations, which would change the paths of the
// project-level resources (metrics, sinks, exclusions).
var LoggingViewAPI = base.APIConfig{
	BaseURL:     "https://logging.googleapis.com/v2",
	APIVersion:  "v2",
	PathBuilder: loggingLocationPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// LoggingViewOperations - as LoggingOperations, but the native ID is taken from
// the response's fully-qualified "name" rather than rebuilt as
// projects/{p}/{type}/{name}. A view's path carries its location and bucket, so
// the flat form would not round-trip through parseLoggingViewNativeID.
var LoggingViewOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractLoggingViewNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// extractLoggingViewNativeID prefers the response's full resource name and
// falls back to composing the path from context (the response is empty on
// delete).
func extractLoggingViewNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name, ok := response["name"].(string); ok {
		if i := strings.Index(name, "projects/"); i >= 0 {
			return name[i:]
		}
	}
	if ctx.ResourceName == "" || ctx.ParentResource == "" {
		return ""
	}
	location := ctx.Location
	if location == "" {
		location = "global"
	}
	return fmt.Sprintf("projects/%s/locations/%s/buckets/%s/views/%s",
		ctx.Project, location, ctx.ParentResource, ctx.ResourceName)
}

// LoggingViewNativeID parses the 8-segment view path.
var LoggingViewNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseLoggingViewNativeID,
}

// loggingLocationPathBuilder builds
// /projects/{project}/locations/{location}/buckets/{bucket}/views[/{name}].
func loggingLocationPathBuilder(ctx base.PathContext) string {
	location := ctx.Location
	if location == "" {
		location = "global"
	}
	// Log scopes exist only in the global location - the API rejects anything
	// else outright ("The location europe-central2 is not supported by
	// logScopes, which may only be global"), including the "-" wildcard. A
	// target configured with a region would otherwise send discovery's List to a
	// location that can never hold a scope, so the resource would be manageable
	// but never discoverable.
	if ctx.ResourceType == "logScopes" {
		location = "global"
	}
	path := fmt.Sprintf("/projects/%s/locations/%s", ctx.Project, location)
	if ctx.ParentResource != "" {
		path += fmt.Sprintf("/%s/%s", ctx.ParentType, ctx.ParentResource)
	}
	path += "/" + ctx.ResourceType
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// parseLoggingViewNativeID parses
// "projects/{p}/locations/{loc}/buckets/{bucket}/views/{id}".
func parseLoggingViewNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 8 || parts[0] != "projects" || parts[2] != "locations" ||
		parts[4] != "buckets" || parts[6] != "views" {
		return base.PathContext{}, fmt.Errorf("invalid Logging view native ID: %s", nativeID)
	}
	return base.PathContext{
		Project:        parts[1],
		Location:       parts[3],
		ParentType:     "buckets",
		ParentResource: parts[5],
		ResourceType:   parts[6],
		ResourceName:   parts[7],
	}, nil
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
