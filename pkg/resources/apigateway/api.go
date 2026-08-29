// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package apigateway

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
)

// APIGatewayAPI - API Gateway API v1. Every write is a long-running operation.
var APIGatewayAPI = base.APIConfig{
	BaseURL:     "https://apigateway.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: apiGatewayPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// APIGatewayOperations - create, patch and delete all answer with an Operation
// to poll rather than the resource.
var APIGatewayOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   func(r map[string]interface{}) string { return utils.GetString(r, "name") },
	OperationURLBuilder:    func(_ base.PathContext, operationID string) string { return operationID },
	NativeIDExtractor:      extractAPIGatewayNativeID,
	OperationStatusChecker: base.CheckLROStatus,

	// Deleting an api while it still holds a config is refused outright:
	// "has nested resources. If the API supports cascading delete, set 'force'
	// to true". A destroy removes the config first, but both deletes are
	// long-running and the api's can reach the API before the config's has
	// finished releasing it. Treat it as retryable so the destroy re-issues the
	// api delete until the config is gone, rather than forcing a cascade that
	// would take resources the forma never asked to remove.
	RetryableError: func(err error) bool {
		return err != nil && strings.Contains(err.Error(), "has nested resources")
	},
}

// APIGatewayNativeID - the full resource path, which is what the API reports as
// "name":
//
//	projects/{p}/locations/global/apis/{api}[/configs/{config}]
//	projects/{p}/locations/{location}/gateways/{gateway}
var APIGatewayNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseAPIGatewayNativeID,
}

// globalLocation - apis and their configs are not regional; only gateways are.
const globalLocation = "global"

// apiGatewayPathBuilder builds the two hierarchies this API has: an api with its
// configs underneath, always global, and a regional gateway alongside them.
func apiGatewayPathBuilder(ctx base.PathContext) string {
	location := globalLocation
	if ctx.ResourceType == "gateways" {
		location = ctx.Location
	}
	path := fmt.Sprintf("/projects/%s/locations/%s", ctx.Project, location)
	if ctx.ParentResource != "" {
		path += "/apis/" + ctx.ParentResource
	}
	path += "/" + ctx.ResourceType
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
		// A config's OpenAPI documents are only returned under the full view.
		// Without them a read reports a config missing the very thing that
		// defines it, and the documents cannot be recovered from anywhere else.
		if ctx.ResourceType == "configs" {
			path += "?view=FULL"
		}
	}
	return path
}

// extractAPIGatewayNativeID reads the resource path. A fresh operation does not
// carry the resource, but its metadata names the target it is building.
func extractAPIGatewayNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name := utils.GetString(response, "name"); name != "" && !strings.Contains(name, "/operations/") {
		return name
	}
	if metadata, ok := response["metadata"].(map[string]interface{}); ok {
		if target := utils.GetString(metadata, "target"); target != "" {
			return target
		}
	}
	if ctx.ResourceName == "" {
		return ""
	}
	return strings.TrimPrefix(apiGatewayPathBuilder(ctx), "/")
}

// parseAPIGatewayNativeID turns the full path back into the context the URL
// builder needs.
func parseAPIGatewayNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" {
		return base.PathContext{}, fmt.Errorf("invalid api gateway native ID: %s", nativeID)
	}
	ctx := base.PathContext{Project: parts[1], Location: parts[3]}
	switch {
	case len(parts) == 6 && parts[4] == "apis":
		ctx.ResourceType = "apis"
		ctx.ResourceName = parts[5]
	case len(parts) == 6 && parts[4] == "gateways":
		ctx.ResourceType = "gateways"
		ctx.ResourceName = parts[5]
	case len(parts) == 8 && parts[4] == "apis" && parts[6] == "configs":
		ctx.ParentResource = parts[5]
		ctx.ResourceType = "configs"
		ctx.ResourceName = parts[7]
	default:
		return base.PathContext{}, fmt.Errorf("invalid api gateway native ID: %s", nativeID)
	}
	return ctx, nil
}
