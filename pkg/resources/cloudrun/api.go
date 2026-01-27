// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package cloudrun

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// CloudRunAPI configuration for GCP Cloud Run API v2
var CloudRunAPI = base.APIConfig{
	BaseURL:     "https://run.googleapis.com/v2",
	APIVersion:  "v2",
	PathBuilder: cloudRunPathBuilder,
	Pagination: &base.PaginationConfig{
		PageSizeParam: "pageSize", // Cloud Run API uses pageSize, not maxResults
	},
}

// CloudRunOperations configuration for Cloud Run operations
var CloudRunOperations = base.OperationConfig{
	Synchronous:            false, // Cloud Run operations are asynchronous (LRO)
	OperationIDExtractor:   extractCloudRunOperationID,
	OperationURLBuilder:    buildCloudRunOperationURL,
	NativeIDExtractor:      extractCloudRunNativeID,
	OperationStatusChecker: checkCloudRunOperationStatus,
}

// CloudRunNativeID configuration for Cloud Run native IDs
var CloudRunNativeID = base.NativeIDConfig{
	Format:       base.FullPathFormat, // Cloud Run uses paths like "projects/my-project/locations/us-central1/services/my-service"
	PathTemplate: "projects/{project}/locations/{location}/{resourceType}/{name}",
	Parser:       parseCloudRunNativeID,
}

// cloudRunPathBuilder builds Cloud Run API paths with location-based scoping
// Cloud Run v2 API format: /projects/{project}/locations/{location}/{resourceType}[/{name}]
// Special case for Create: adds query parameter ?serviceId={name} or ?jobId={name}
// Location must be explicitly provided in target config (no wildcards or defaults).
func cloudRunPathBuilder(ctx base.PathContext) string {
	// Use location (Cloud Run v2 uses locations, not zones/regions)
	// Location must be explicitly set in target config
	location := ctx.Location

	// Build base path
	path := fmt.Sprintf("/projects/%s/locations/%s/%s", ctx.Project, location, ctx.ResourceType)

	// For specific resource operations (Read, Delete, Status), append the resource name
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}

	return path
}

// extractCloudRunOperationID extracts operation name from Cloud Run API response
// Cloud Run returns LRO (Long Running Operation) with a "name" field
func extractCloudRunOperationID(response map[string]interface{}) string {
	// Cloud Run returns operation name in the "name" field
	// Format: "projects/{project}/locations/{location}/operations/{operation-id}"
	if name, ok := response["name"].(string); ok {
		return name
	}
	return ""
}

// buildCloudRunOperationURL constructs the URL to check operation status
// Cloud Run operation URLs are the full operation name path
func buildCloudRunOperationURL(ctx base.PathContext, operationID string) string {
	// operationID is already the full path: "projects/{project}/locations/{location}/operations/{operation-id}"
	// Just return it as-is (baseURL will be prepended by Status method)
	return operationID
}

// extractCloudRunNativeID extracts the native ID (full path) from Cloud Run API response
func extractCloudRunNativeID(response map[string]interface{}, ctx base.PathContext) string {
	// For completed operations, check if there's a response field with the created resource
	if responseField, ok := response["response"].(map[string]interface{}); ok {
		if name, ok := responseField["name"].(string); ok {
			// This is the actual resource path
			return extractPathFromURL(name)
		}
	}

	// For direct resource responses (read/get), extract from name field
	if name, ok := response["name"].(string); ok {
		if !strings.Contains(name, "/operations/") {
			// This is a direct resource response (not an operation)
			return extractPathFromURL(name)
		}
		// If it's an operation response and we have context, construct the path
		if ctx.ResourceName != "" {
			location := ctx.Location
			if location == "" {
				location = "us-central1"
			}
			return fmt.Sprintf("projects/%s/locations/%s/%s/%s",
				ctx.Project, location, ctx.ResourceType, ctx.ResourceName)
		}
	}

	// For operations, try metadata.target (some APIs use this)
	if metadata, ok := response["metadata"].(map[string]interface{}); ok {
		if target, ok := metadata["target"].(string); ok {
			return extractPathFromURL(target)
		}
	}

	return ""
}

// extractPathFromURL extracts the resource path from a full Cloud Run URL
// Example: "https://run.googleapis.com/v2/projects/my-project/locations/us-central1/services/my-service"
// Returns: "projects/my-project/locations/us-central1/services/my-service"
func extractPathFromURL(url string) string {
	// Find "/projects/" which marks the start of the resource path
	idx := strings.Index(url, "/projects/")
	if idx == -1 {
		// Might already be a path without URL prefix
		if strings.HasPrefix(url, "projects/") {
			return url
		}
		return ""
	}
	return url[idx+1:] // Skip the leading slash
}

// parseCloudRunNativeID parses a Cloud Run full-path native ID into PathContext
// Example: "projects/my-project/locations/us-central1/services/my-service"
func parseCloudRunNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 6 {
		return base.PathContext{}, fmt.Errorf("invalid cloud run native ID: %s", nativeID)
	}

	ctx := base.PathContext{}

	// Parse: projects/{project}/locations/{location}/{resource-type}/{name}
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
			// Assume this is the resource type, and next is the name
			if i+1 < len(parts) {
				ctx.ResourceType = parts[i]
				ctx.ResourceName = parts[i+1]
				i++
			}
		}
	}

	return ctx, nil
}

// checkCloudRunOperationStatus checks if a Cloud Run LRO operation is complete
func checkCloudRunOperationStatus(operationResponse map[string]interface{}) (done bool, err error) {
	// Check done field - if missing or not a bool, treat as not done yet
	done, ok := operationResponse["done"].(bool)
	if !ok {
		// Missing 'done' field means operation is still in progress
		return false, nil
	}

	if !done {
		return false, nil
	}

	// Check for errors
	if errorObj, ok := operationResponse["error"].(map[string]interface{}); ok {
		if message, ok := errorObj["message"].(string); ok {
			return true, fmt.Errorf("operation failed: %s", message)
		}
		return true, fmt.Errorf("operation failed with unknown error")
	}

	return true, nil
}
