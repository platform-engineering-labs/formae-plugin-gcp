// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package servicedirectory

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// ServiceDirectoryAPI - Service Directory API v1. Resources are
// location-scoped. namespaces.create/patch return the Namespace directly and
// delete returns Empty, so all operations are synchronous (no LRO polling).
var ServiceDirectoryAPI = base.APIConfig{
	BaseURL:     "https://servicedirectory.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: serviceDirectoryPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize", PageTokenParam: "pageToken"},
}

// ServiceDirectoryOperations - synchronous. create/patch/delete return the
// resource (or Empty) directly, never an Operation.
var ServiceDirectoryOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractServiceDirectoryNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// ServiceDirectoryNativeID - full path
// "projects/{project}/locations/{location}/namespaces/{name}". The default
// path parser handles it.
var ServiceDirectoryNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
}

// serviceDirectoryPathBuilder builds
// /projects/{project}/locations/{location}/{resourceType}[/{name}].
func serviceDirectoryPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s/%s", ctx.Project, ctx.Location, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractServiceDirectoryNativeID returns the full resource path. A synchronous
// create response is the Namespace itself, carrying its full path in "name".
// Fall back to building the path from context when absent.
func extractServiceDirectoryNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name, ok := response["name"].(string); ok && strings.HasPrefix(name, "projects/") {
		return name
	}
	if ctx.ResourceName != "" {
		return fmt.Sprintf("projects/%s/locations/%s/%s/%s",
			ctx.Project, ctx.Location, ctx.ResourceType, ctx.ResourceName)
	}
	return ""
}
