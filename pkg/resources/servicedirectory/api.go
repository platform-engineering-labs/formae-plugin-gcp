// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package servicedirectory

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
)

// ServiceDirectoryAPI - Service Directory API v1. Everything is regional and
// every operation is synchronous: create, patch and delete all answer with the
// resource itself rather than an Operation to poll.
var ServiceDirectoryAPI = base.APIConfig{
	BaseURL:     "https://servicedirectory.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: serviceDirectoryPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// ServiceDirectoryOperations - synchronous, so Status has nothing to poll.
var ServiceDirectoryOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractServiceDirectoryNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// ServiceDirectoryNativeID - the full resource path, which is what the API
// reports as "name":
//
//	projects/{p}/locations/{l}/namespaces/{ns}[/services/{svc}[/endpoints/{ep}]]
var ServiceDirectoryNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseServiceDirectoryNativeID,
}

// serviceDirectoryPathBuilder builds the three levels of the hierarchy. A
// service names its namespace; an endpoint names both its namespace and its
// service, carried as "{namespace}/{service}" the way the two-property parent
// arrives from the properties.
func serviceDirectoryPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s", ctx.Project, ctx.Location)
	if ctx.ParentResource != "" {
		namespace, service, hasService := strings.Cut(ctx.ParentResource, "/")
		path += "/namespaces/" + namespace
		if hasService && service != "" {
			path += "/services/" + service
		}
	}
	path += "/" + ctx.ResourceType
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractServiceDirectoryNativeID takes the full path the API reports.
func extractServiceDirectoryNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name := utils.GetString(response, "name"); name != "" {
		return name
	}
	if ctx.ResourceName == "" {
		return ""
	}
	// A response without a name still has to yield an addressable id, so rebuild
	// it from the context that addressed the request.
	return strings.TrimPrefix(serviceDirectoryPathBuilder(ctx), "/")
}

// parseServiceDirectoryNativeID turns the full path back into the context the
// URL builder needs. The parent is carried as "{namespace}" for a service and
// "{namespace}/{service}" for an endpoint.
func parseServiceDirectoryNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "namespaces" {
		return base.PathContext{}, fmt.Errorf("invalid service directory native ID: %s", nativeID)
	}
	ctx := base.PathContext{
		Project:  parts[1],
		Location: parts[3],
	}
	switch {
	case len(parts) == 6: // .../namespaces/{ns}
		ctx.ResourceType = "namespaces"
		ctx.ResourceName = parts[5]
	case len(parts) == 8 && parts[6] == "services": // .../namespaces/{ns}/services/{svc}
		ctx.ParentResource = parts[5]
		ctx.ResourceType = "services"
		ctx.ResourceName = parts[7]
	case len(parts) == 10 && parts[6] == "services" && parts[8] == "endpoints":
		ctx.ParentResource = parts[5] + "/" + parts[7]
		ctx.ResourceType = "endpoints"
		ctx.ResourceName = parts[9]
	default:
		return base.PathContext{}, fmt.Errorf("invalid service directory native ID: %s", nativeID)
	}
	return ctx, nil
}
