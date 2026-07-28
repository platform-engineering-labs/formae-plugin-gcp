// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package apigateway

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// apiLocation is the fixed location segment for API Gateway apis. Unlike most
// location-scoped GCP resources, apis live only under locations/global — the
// discovery doc pins every apis path to `projects/*/locations/global/apis/*`.
// The path builder and native-ID builder therefore hardcode "global" and
// ignore the target's configured region/location.
const apiLocation = "global"

// APIGatewayAPI - API Gateway API v1. Apis are location-scoped, but the
// location is always "global". create/delete are long-running operations
// (return an Operation to poll); get/list return the resource directly.
var APIGatewayAPI = base.APIConfig{
	BaseURL:     "https://apigateway.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: apiGatewayPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// APIGatewayOperations - asynchronous (LRO). create/delete return an
// Operation; formae polls Status() until the operation reports done.
var APIGatewayOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractAPINativeID,
	OperationStatusChecker: checkOperationStatus,
}

// APIGatewayNativeID - full path
// "projects/{project}/locations/global/apis/{name}".
var APIGatewayNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
}

// apiGatewayPathBuilder builds
// /projects/{project}/locations/global/{resourceType}[/{name}]. The location
// segment is always "global" for API Gateway apis (see apiLocation).
func apiGatewayPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s/%s", ctx.Project, apiLocation, ctx.ResourceType)
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

// extractAPINativeID builds the resource path. On async create the response is
// an Operation (not the resource), so build from context — where
// buildPathContext has already set ResourceName from the declared id. The
// location is always "global". Fall back to the operation's metadata.target.
func extractAPINativeID(response map[string]interface{}, ctx base.PathContext) string {
	if ctx.ResourceName != "" {
		return fmt.Sprintf("projects/%s/locations/%s/%s/%s",
			ctx.Project, apiLocation, ctx.ResourceType, ctx.ResourceName)
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
