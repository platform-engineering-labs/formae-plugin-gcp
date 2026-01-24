// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/base"
)

// ComputeAPI configuration for GCP Compute Engine API
var ComputeAPI = base.APIConfig{
	BaseURL:     "https://compute.googleapis.com/compute/v1",
	APIVersion:  "v1",
	PathBuilder: computePathBuilder,
}

// ComputeOperations configuration for Compute Engine operations
var ComputeOperations = base.OperationConfig{
	Synchronous:            false, // Compute operations are asynchronous
	OperationIDExtractor:   extractComputeOperationID,
	OperationURLBuilder:    buildComputeOperationURL,
	NativeIDExtractor:      extractComputeNativeID,
	OperationStatusChecker: checkComputeOperationStatus,
}

// ComputeNativeID configuration for Compute Engine native IDs
var ComputeNativeID = base.NativeIDConfig{
	Format:       base.FullPathFormat, // Compute uses full paths like "projects/my-project/global/networks/my-network"
	PathTemplate: "", // Path template dynamically constructed based on scope
	Parser:       parseComputeNativeID,
}

// computePathBuilder builds Compute API paths with global/regional/zonal scoping
func computePathBuilder(ctx base.PathContext) string {
	// Determine scope
	var scope string

	if ctx.Zone != "" {
		scope = fmt.Sprintf("zones/%s", ctx.Zone)
	} else if ctx.Region != "" {
		scope = fmt.Sprintf("regions/%s", ctx.Region)
	} else {
		scope = "global"
	}

	// Build path
	path := fmt.Sprintf("/projects/%s/%s/%s", ctx.Project, scope, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}

	return path
}

// extractComputeOperationID extracts operation ID from Compute API response
func extractComputeOperationID(response map[string]interface{}) string {
	// Compute returns operation in multiple possible formats:
	// 1. selfLink: full URL to operation
	// 2. name: just the operation name
	// 3. targetLink: link to the resource being created/updated/deleted

	if selfLink, ok := response["selfLink"].(string); ok {
		// Extract operation name from selfLink
		parts := strings.Split(selfLink, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	if name, ok := response["name"].(string); ok {
		return name
	}

	return ""
}

// buildComputeOperationURL constructs the URL to check operation status
func buildComputeOperationURL(ctx base.PathContext, operationID string) string {
	// Return just the path (baseURL will be prepended by Status method)
	// Determine scope for operation URL
	if ctx.Zone != "" {
		return fmt.Sprintf("projects/%s/zones/%s/operations/%s", ctx.Project, ctx.Zone, operationID)
	} else if ctx.Region != "" {
		return fmt.Sprintf("projects/%s/regions/%s/operations/%s", ctx.Project, ctx.Region, operationID)
	}

	// Global operation
	return fmt.Sprintf("projects/%s/global/operations/%s", ctx.Project, operationID)
}

// extractComputeNativeID extracts the native ID (full path) from Compute API response
func extractComputeNativeID(response map[string]interface{}, ctx base.PathContext) string {
	// For operations (create/update/delete responses), extract from targetLink first
	if targetLink, ok := response["targetLink"].(string); ok {
		// Extract the path portion after /compute/v1/
		return extractPathFromURL(targetLink)
	}

	// For direct resource responses (read/list), use selfLink
	if selfLink, ok := response["selfLink"].(string); ok {
		return extractPathFromURL(selfLink)
	}

	// Fallback: construct from name field if available
	if name, ok := response["name"].(string); ok {
		// Need to construct full path using context
		return computePathBuilder(base.PathContext{
			Project:      ctx.Project,
			Region:       ctx.Region,
			Zone:         ctx.Zone,
			ResourceType: ctx.ResourceType,
			ResourceName: name,
		})[1:] // Remove leading slash
	}

	return ""
}

// extractPathFromURL extracts the resource path from a full Compute URL
// Example: "https://www.googleapis.com/compute/v1/projects/my-project/global/networks/my-network"
// Returns: "projects/my-project/global/networks/my-network"
func extractPathFromURL(url string) string {
	// Find "/projects/" which marks the start of the resource path
	idx := strings.Index(url, "/projects/")
	if idx == -1 {
		return ""
	}
	return url[idx+1:] // Skip the leading slash
}

// parseComputeNativeID parses a Compute full-path native ID into PathContext
// Example: "projects/my-project/global/networks/my-network"
// Example: "projects/my-project/regions/us-central1/addresses/my-address"
func parseComputeNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 4 {
		return base.PathContext{}, fmt.Errorf("invalid compute native ID: %s", nativeID)
	}

	ctx := base.PathContext{}

	// Parse: projects/{project}/{scope}/{resource-type}/{name}
	for i := 0; i < len(parts); i++ {
		switch parts[i] {
		case "projects":
			if i+1 < len(parts) {
				ctx.Project = parts[i+1]
				i++
			}
		case "global":
			// Global scope - no region/zone
			continue
		case "regions":
			if i+1 < len(parts) {
				ctx.Region = parts[i+1]
				i++
			}
		case "zones":
			if i+1 < len(parts) {
				ctx.Zone = parts[i+1]
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

// checkComputeOperationStatus checks if a Compute operation is complete
func checkComputeOperationStatus(operationResponse map[string]interface{}) (done bool, err error) {
	// Check status field
	status, ok := operationResponse["status"].(string)
	if !ok {
		return false, fmt.Errorf("operation response missing status field")
	}

	// Operation is done when status is "DONE"
	if status != "DONE" {
		return false, nil
	}

	// Check for errors
	if errorObj, ok := operationResponse["error"].(map[string]interface{}); ok {
		if errors, ok := errorObj["errors"].([]interface{}); ok && len(errors) > 0 {
			if firstErr, ok := errors[0].(map[string]interface{}); ok {
				if msg, ok := firstErr["message"].(string); ok {
					return true, fmt.Errorf("operation failed: %s", msg)
				}
			}
		}
		return true, fmt.Errorf("operation failed with unknown error")
	}

	return true, nil
}
