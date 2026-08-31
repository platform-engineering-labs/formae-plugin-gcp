// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package servicedirectory implements GCP Service Directory resources.
package servicedirectory

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// ServiceDirectoryAPI - Service Directory API v1. Every resource is
// location-scoped and the three collections nest: a service lives under a
// namespace, an endpoint under a service.
var ServiceDirectoryAPI = base.APIConfig{
	BaseURL:     "https://servicedirectory.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: serviceDirectoryPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// ServiceDirectoryOperations - synchronous. Create returns the created resource
// and delete an empty body; there is no operation to poll.
var ServiceDirectoryOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractServiceDirectoryNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// ServiceDirectoryNativeID - the full resource path, in one of three shapes:
//
//	projects/{p}/locations/{l}/namespaces/{ns}
//	projects/{p}/locations/{l}/namespaces/{ns}/services/{svc}
//	projects/{p}/locations/{l}/namespaces/{ns}/services/{svc}/endpoints/{ep}
var ServiceDirectoryNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseServiceDirectoryNativeID,
}

// serviceDirectoryPathBuilder builds
// /projects/{p}/locations/{l}[/namespaces/{ns}][/services/{svc}]/{resourceType}[/{name}].
//
// The immediate parent comes from ParentResource and, for endpoints, the
// namespace above it from CustomSegments[0].
func serviceDirectoryPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s", ctx.Project, ctx.Location)
	if len(ctx.CustomSegments) > 0 && ctx.CustomSegments[0] != "" {
		path += "/namespaces/" + ctx.CustomSegments[0]
	}
	if ctx.ParentType != "" && ctx.ParentResource != "" {
		path += fmt.Sprintf("/%s/%s", ctx.ParentType, ctx.ParentResource)
	}
	path += "/" + ctx.ResourceType
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// parseServiceDirectoryNativeID turns a native ID back into the context the URL
// builder needs. A nested resource has to restore its parents, or a read would
// address the location-level collection and 404.
func parseServiceDirectoryNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "namespaces" {
		return base.PathContext{}, fmt.Errorf("invalid service directory native ID: %s", nativeID)
	}
	ctx := base.PathContext{
		Project:      parts[1],
		Location:     parts[3],
		ResourceType: parts[4],
		ResourceName: parts[5],
	}
	switch len(parts) {
	case 6: // namespace
	case 8: // service: .../namespaces/{ns}/services/{svc}
		if parts[6] != "services" {
			return base.PathContext{}, fmt.Errorf("invalid service directory native ID: %s", nativeID)
		}
		ctx.ParentType = "namespaces"
		ctx.ParentResource = parts[5]
		ctx.ResourceType = parts[6]
		ctx.ResourceName = parts[7]
	case 10: // endpoint: .../namespaces/{ns}/services/{svc}/endpoints/{ep}
		if parts[6] != "services" || parts[8] != "endpoints" {
			return base.PathContext{}, fmt.Errorf("invalid service directory native ID: %s", nativeID)
		}
		ctx.CustomSegments = []string{parts[5]}
		ctx.ParentType = "services"
		ctx.ParentResource = parts[7]
		ctx.ResourceType = parts[8]
		ctx.ResourceName = parts[9]
	default:
		return base.PathContext{}, fmt.Errorf("invalid service directory native ID: %s", nativeID)
	}
	return ctx, nil
}

// extractServiceDirectoryNativeID prefers the full path the API echoes in
// "name" - that is already the native ID shape, and it is the only source a
// List item has. Falls back to the request context, which create fills in.
func extractServiceDirectoryNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name, ok := response["name"].(string); ok {
		if i := strings.Index(name, "projects/"); i >= 0 {
			return name[i:]
		}
	}
	if ctx.ResourceName == "" {
		return ""
	}
	parents := ""
	if len(ctx.CustomSegments) > 0 && ctx.CustomSegments[0] != "" {
		parents += "namespaces/" + ctx.CustomSegments[0] + "/"
	}
	if ctx.ParentType != "" && ctx.ParentResource != "" {
		parents += ctx.ParentType + "/" + ctx.ParentResource + "/"
	}
	return fmt.Sprintf("projects/%s/locations/%s/%s%s/%s",
		ctx.Project, ctx.Location, parents, ctx.ResourceType, ctx.ResourceName)
}
