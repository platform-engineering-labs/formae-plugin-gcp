// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package gkehub

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// GKEHubAPI configuration for GCP GKE Hub API v1
var GKEHubAPI = base.APIConfig{
	BaseURL:     "https://gkehub.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: gkehubPathBuilder,
	Pagination: &base.PaginationConfig{
		PageSizeParam: "pageSize",
	},
}

// GKEHubOperations configuration for GKE Hub operations (LRO-based)
var GKEHubOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   extractOperationID,
	OperationURLBuilder:    buildOperationURL,
	NativeIDExtractor:      extractNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// GKEHubNativeID configuration for GKE Hub native IDs
var GKEHubNativeID = base.NativeIDConfig{
	Format:       base.FullPathFormat,
	PathTemplate: "projects/{project}/locations/{location}/memberships/{name}",
	Parser:       parseNativeID,
}

// gkehubPathBuilder builds GKE Hub API paths
// Format: /projects/{project}/locations/{location}/memberships[/{name}]
func gkehubPathBuilder(ctx base.PathContext) string {
	location := ctx.Location
	if location == "" {
		location = "global"
	}

	path := fmt.Sprintf("/projects/%s/locations/%s/%s", ctx.Project, location, ctx.ResourceType)

	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}

	return path
}

func extractOperationID(response map[string]interface{}) string {
	if name, ok := response["name"].(string); ok {
		return name
	}
	return ""
}

func buildOperationURL(_ base.PathContext, operationID string) string {
	return operationID
}

func extractNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if responseField, ok := response["response"].(map[string]interface{}); ok {
		if name, ok := responseField["name"].(string); ok {
			return extractPathFromURL(name)
		}
	}

	if name, ok := response["name"].(string); ok {
		if !strings.Contains(name, "/operations/") {
			return extractPathFromURL(name)
		}
		if ctx.ResourceName != "" {
			location := ctx.Location
			if location == "" {
				location = "global"
			}
			return fmt.Sprintf("projects/%s/locations/%s/%s/%s",
				ctx.Project, location, ctx.ResourceType, ctx.ResourceName)
		}
	}

	if metadata, ok := response["metadata"].(map[string]interface{}); ok {
		if target, ok := metadata["target"].(string); ok {
			return extractPathFromURL(target)
		}
	}

	return ""
}

func extractPathFromURL(url string) string {
	idx := strings.Index(url, "/projects/")
	if idx == -1 {
		if strings.HasPrefix(url, "projects/") {
			return url
		}
		return ""
	}
	return url[idx+1:]
}

func parseNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 6 {
		return base.PathContext{}, fmt.Errorf("invalid gke hub native ID: %s", nativeID)
	}

	ctx := base.PathContext{}

	for i := 0; i < len(parts); i++ {
		switch parts[i] {
		case "projects":
			if i+1 < len(parts) {
				ctx.Project = parts[i+1]
				i++
			}
		case "locations":
			if i+1 < len(parts) {
				ctx.Location = parts[i+1]
				i++
			}
		default:
			if i+1 < len(parts) {
				ctx.ResourceType = parts[i]
				ctx.ResourceName = parts[i+1]
				i++
			}
		}
	}

	return ctx, nil
}

func checkOperationStatus(operationResponse map[string]interface{}) (bool, error) {
	done, ok := operationResponse["done"].(bool)
	if !ok {
		return false, nil
	}

	if !done {
		return false, nil
	}

	if errorObj, ok := operationResponse["error"].(map[string]interface{}); ok {
		if message, ok := errorObj["message"].(string); ok {
			return true, fmt.Errorf("operation failed: %s", message)
		}
		return true, fmt.Errorf("operation failed with unknown error")
	}

	return true, nil
}
