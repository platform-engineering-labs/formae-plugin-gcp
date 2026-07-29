// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package networkconnectivity

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// location is pinned to "global": Network Connectivity hubs live under
// projects/{p}/locations/global/hubs and are not regional. The path and
// native-ID builders use this const and deliberately ignore ctx.Location so a
// target's configured region can never leak into a hub URL.
const location = "global"

// NetworkConnectivityAPI - Network Connectivity API v1. Hubs are global
// resources. create/delete are long-running operations (return an Operation to
// poll); get/list return the resource directly.
var NetworkConnectivityAPI = base.APIConfig{
	BaseURL:     "https://networkconnectivity.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: networkConnectivityPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// NetworkConnectivityOperations - asynchronous (LRO). create/delete return an
// Operation; formae polls Status() until the operation reports done.
var NetworkConnectivityOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractNetworkConnectivityNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// NetworkConnectivityNativeID - full path
// "projects/{project}/locations/global/hubs/{name}".
var NetworkConnectivityNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
}

// networkConnectivityPathBuilder builds
// /projects/{project}/locations/global/{resourceType}[/{name}]. The location is
// hard-pinned to "global"; ctx.Location is intentionally ignored.
func networkConnectivityPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s/%s", ctx.Project, location, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractOperationName returns the LRO operation name from a create/delete
// response ("projects/{p}/locations/global/operations/{op}"). base.Status GETs
// BaseURL + "/" + this to poll.
func extractOperationName(response map[string]interface{}) string {
	if name, ok := response["name"].(string); ok && strings.Contains(name, "/operations/") {
		return name
	}
	return ""
}

// extractNetworkConnectivityNativeID builds the resource path. On async create
// the response is an Operation (not the resource), so build from context —
// buildPathContext has already set ResourceName from the declared id. The
// location is hard-pinned to "global"; ctx.Location is ignored. Fall back to the
// operation's metadata.target, then to a direct resource response.
func extractNetworkConnectivityNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if ctx.ResourceName != "" {
		return fmt.Sprintf("projects/%s/locations/%s/%s/%s",
			ctx.Project, location, ctx.ResourceType, ctx.ResourceName)
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
