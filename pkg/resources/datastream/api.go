// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package datastream

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// DatastreamAPI - Datastream API v1. Resources are location-scoped.
// create/delete are long-running operations (return an Operation to poll);
// get/list return the resource directly.
var DatastreamAPI = base.APIConfig{
	BaseURL:     "https://datastream.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: datastreamPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// DatastreamOperations - asynchronous (LRO). create/delete return an
// Operation; formae polls Status() until the operation reports done.
var DatastreamOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractDatastreamNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// DatastreamNativeID - full path
// "projects/{project}/locations/{location}/connectionProfiles/{name}".
var DatastreamNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
}

// datastreamPathBuilder builds
// /projects/{project}/locations/{location}/{resourceType}[/{name}].
func datastreamPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s/%s", ctx.Project, ctx.Location, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractOperationName returns the LRO operation name from a create/delete
// response ("projects/{p}/locations/{l}/operations/{op}"). base.Status GETs
// BaseURL + "/" + this to poll.
func extractOperationName(response map[string]interface{}) string {
	if name, ok := response["name"].(string); ok && strings.Contains(name, "/operations/") {
		return name
	}
	return ""
}

// extractDatastreamNativeID builds the resource path. On async create the
// response is an Operation (not the resource), so build from context — where
// buildPathContext has already set ResourceName from the declared id. Fall back
// to the operation's metadata.target.
func extractDatastreamNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if ctx.ResourceName != "" {
		return fmt.Sprintf("projects/%s/locations/%s/%s/%s",
			ctx.Project, ctx.Location, ctx.ResourceType, ctx.ResourceName)
	}
	if md, ok := response["metadata"].(map[string]interface{}); ok {
		if target, ok := md["target"].(string); ok {
			if i := strings.Index(target, "projects/"); i >= 0 {
				return target[i:]
			}
		}
	}
	// Direct resource response (get): "name" is the full path.
	if name, ok := response["name"].(string); ok && !strings.Contains(name, "/operations/") {
		if i := strings.Index(name, "projects/"); i >= 0 {
			return name[i:]
		}
	}
	return ""
}

// checkOperationStatus reports whether a polled Operation is done, mapping a
// present "error" to a terminal failure.
func checkOperationStatus(op map[string]interface{}) (bool, error) {
	done, _ := op["done"].(bool)
	if !done {
		return false, nil
	}
	if errObj, ok := op["error"].(map[string]interface{}); ok {
		msg, _ := errObj["message"].(string)
		if msg == "" {
			msg = "operation failed"
		}
		return true, fmt.Errorf("%s", msg)
	}
	return true, nil
}
