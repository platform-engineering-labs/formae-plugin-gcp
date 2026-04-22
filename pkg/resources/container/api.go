// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package container

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
)

// ContainerAPI defines the API configuration for GCP Container (GKE) API
var ContainerAPI = base.APIConfig{
	BaseURL:     "https://container.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: containerPathBuilder,
	Pagination: &base.PaginationConfig{
		Disabled: true, // Container API doesn't support pagination query parameters
	},
}

// ContainerOperations defines how operations work in the Container API
var ContainerOperations = base.OperationConfig{
	Synchronous: false, // Container operations are asynchronous

	// Extract operation name from response
	OperationIDExtractor: func(response map[string]interface{}) string {
		opName := utils.GetString(response, "name")
		// Operation name might be a full path, extract just the name
		return ExtractOperationNameFromSelfLink(opName)
	},

	// Build operation URL: projects/{project}/locations/{location}/operations/{operation}
	OperationURLBuilder: func(ctx base.PathContext, operationID string) string {
		return fmt.Sprintf("projects/%s/locations/%s/operations/%s", ctx.Project, ctx.Location, operationID)
	},

	// Extract native ID from response
	NativeIDExtractor: extractContainerNativeID,

	// Check if operation is complete
	OperationStatusChecker: func(response map[string]interface{}) (bool, error) {
		status := utils.GetString(response, "status")
		isDone := status == "DONE"

		// Check for errors in the operation
		if isDone {
			if errorMsg := utils.GetString(response, "error"); errorMsg != "" {
				return true, fmt.Errorf("operation failed: %s", errorMsg)
			}
		}

		return isDone, nil
	},
}

// ContainerNativeID defines native ID format for Container resources
var ContainerNativeID = base.NativeIDConfig{
	Format:       base.FullPathFormat, // Path-only format
	PathTemplate: "projects/{project}/locations/{location}/{resourceType}/{name}",
	Parser:       parseContainerNativeID,
}

// containerPathBuilder builds Container API paths
// Format: /projects/{project}/locations/{location}/{resourceType}/{name}
// For nested resources: /projects/{project}/locations/{location}/clusters/{cluster}/nodePools/{nodePool}
func containerPathBuilder(ctx base.PathContext) string {
	parentPath := fmt.Sprintf("/projects/%s/locations/%s", ctx.Project, ctx.Location)

	// Handle nested resources (e.g., nodePools under clusters)
	// For nested resources, ParentResource contains the parent name (e.g., cluster name)
	// and ParentType contains the parent type (e.g., "clusters")
	if ctx.ParentResource != "" && ctx.ParentType != "" {
		// Nested resource: .../clusters/{cluster}/nodePools/{nodePool}
		if ctx.ResourceName != "" {
			return fmt.Sprintf("%s/%s/%s/%s/%s", parentPath, ctx.ParentType, ctx.ParentResource, ctx.ResourceType, ctx.ResourceName)
		}
		return fmt.Sprintf("%s/%s/%s/%s", parentPath, ctx.ParentType, ctx.ParentResource, ctx.ResourceType)
	}

	// Top-level resource: .../clusters/{cluster}
	if ctx.ResourceName != "" {
		return fmt.Sprintf("%s/%s/%s", parentPath, ctx.ResourceType, ctx.ResourceName)
	}
	return fmt.Sprintf("%s/%s", parentPath, ctx.ResourceType)
}

// extractContainerNativeID extracts the native ID (path-only format) from Container API response
func extractContainerNativeID(response map[string]interface{}, ctx base.PathContext) string {
	// Check if this is an operation response (has "operationType" or "status" field)
	if _, hasOpType := response["operationType"]; hasOpType {
		// This is an operation response, try targetLink first
		if targetLink, ok := response["targetLink"].(string); ok && targetLink != "" {
			return extractPathFromURL(targetLink)
		}
		// If no targetLink, we can't determine the resource ID from the operation
		// Return empty and let it be populated after the operation completes
		return ""
	}

	// For resource responses, try selfLink first (most accurate)
	if selfLink, ok := response["selfLink"].(string); ok && selfLink != "" {
		return extractPathFromURL(selfLink)
	}

	// Fallback: extract name and build path
	name := utils.GetString(response, "name")
	if name == "" {
		return ""
	}

	// Don't try to build path if name looks like an operation path
	if strings.Contains(name, "/operations/") {
		return ""
	}

	// Build path-only native ID
	parentPath := fmt.Sprintf("projects/%s/locations/%s", ctx.Project, ctx.Location)

	// Handle nested resources
	if ctx.ParentType != "" && ctx.ParentResource != "" {
		return fmt.Sprintf("%s/%s/%s/%s/%s", parentPath, ctx.ParentType, ctx.ParentResource, ctx.ResourceType, name)
	}

	// Top-level resource
	return fmt.Sprintf("%s/%s/%s", parentPath, ctx.ResourceType, name)
}

// extractPathFromURL extracts the path portion from a full Container API URL
// Example: "https://container.googleapis.com/v1/projects/..." -> "projects/..."
func extractPathFromURL(urlStr string) string {
	// Remove protocol and domain
	path := urlStr
	if idx := strings.Index(urlStr, "//"); idx != -1 {
		path = urlStr[idx+2:]
		// Skip the domain
		if idx := strings.Index(path, "/"); idx != -1 {
			path = path[idx+1:]
		}
	}
	// Remove API version if present
	path = strings.TrimPrefix(path, "v1/")
	return path
}

// parseContainerNativeID parses a Container native ID (full URL) into PathContext
// Format: https://container.googleapis.com/v1/projects/{project}/locations/{location}/clusters/{cluster}
// Or nested: https://container.googleapis.com/v1/projects/{project}/locations/{location}/clusters/{cluster}/nodePools/{nodePool}
func parseContainerNativeID(nativeID string) (base.PathContext, error) {
	// Handle both full URLs and path-only formats
	path := nativeID

	// If it's a full URL, extract the path portion
	if strings.HasPrefix(nativeID, "https://") || strings.HasPrefix(nativeID, "http://") {
		// Find the path after the domain
		if idx := strings.Index(nativeID, "//"); idx != -1 {
			path = nativeID[idx+2:] // Skip past "//"
			// Skip the domain (everything before the next "/")
			if idx := strings.Index(path, "/"); idx != -1 {
				path = path[idx+1:] // Start from the path after domain
			}
		}
		// Remove API version if present
		path = strings.TrimPrefix(path, "v1/")
	}

	// Parse path: projects/{project}/(locations|zones)/{location}/clusters/{cluster}[/nodePools/{nodePool}]
	// GCP's Container API returns selfLink with "zones/" for zonal clusters and "locations/"
	// for regional clusters. Both are valid inputs to the v1 REST API (which prefers
	// "locations/" but accepts legacy "zones/" paths for zonal resources).
	parts := strings.Split(path, "/")
	if len(parts) < 6 || parts[0] != "projects" || (parts[2] != "locations" && parts[2] != "zones") {
		return base.PathContext{}, fmt.Errorf("invalid Container native ID format: %s", nativeID)
	}

	ctx := base.PathContext{
		Project:  parts[1],
		Location: parts[3],
	}

	// Check if it's a nested resource (has more than 6 parts)
	if len(parts) >= 8 {
		// Nested resource: projects/{project}/locations/{location}/clusters/{cluster}/nodePools/{nodePool}
		ctx.ParentType = parts[4]     // e.g., "clusters"
		ctx.ParentResource = parts[5] // cluster name
		ctx.ResourceType = parts[6]   // e.g., "nodePools"
		if len(parts) > 7 {
			ctx.ResourceName = parts[7] // nodePool name
		}
	} else {
		// Top-level resource: projects/{project}/locations/{location}/clusters/{cluster}
		ctx.ResourceType = parts[4] // e.g., "clusters"
		if len(parts) > 5 {
			ctx.ResourceName = parts[5] // cluster name
		}
	}

	return ctx, nil
}
