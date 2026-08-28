// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigtable

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// BigtableAPI configuration for GCP Bigtable Admin API v2
var BigtableAPI = base.APIConfig{
	BaseURL:     "https://bigtableadmin.googleapis.com/v2",
	APIVersion:  "v2",
	PathBuilder: bigtablePathBuilder,
	Pagination: &base.PaginationConfig{
		Disabled: true, // Container API doesn't support pagination query parameters
	},
}

// BigtableOperations configuration for Bigtable operations
var BigtableOperations = base.OperationConfig{
	Synchronous:            false, // Bigtable operations are asynchronous (LRO)
	OperationIDExtractor:   extractBigtableOperationID,
	OperationURLBuilder:    buildBigtableOperationURL,
	NativeIDExtractor:      extractBigtableNativeID,
	OperationStatusChecker: checkBigtableOperationStatus,
}

// BigtableSyncOperations - for the collections whose create answers with the
// resource itself rather than an Operation. appProfiles and tables do; clusters,
// logicalViews and materializedViews return an Operation to poll.
var BigtableSyncOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractBigtableNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// BigtableNativeID configuration for Bigtable native IDs
var BigtableNativeID = base.NativeIDConfig{
	Format:       base.FullPathFormat,
	PathTemplate: "projects/{project}/instances/{instance}[/clusters/{cluster}][/tables/{table}][/backups/{backup}][/materializedViews/{materializedView}]",
	Parser:       parseBigtableNativeID,
}

// bigtablePathBuilder builds Bigtable Admin API paths
// Bigtable API format:
// - Instances: /projects/{project}/instances[/{instance}]
// - Clusters: /projects/{project}/instances/{instance}/clusters[/{cluster}]
// - Tables: /projects/{project}/instances/{instance}/tables[/{table}]
// - Backups: /projects/{project}/instances/{instance}/clusters/{cluster}/backups[/{backup}]
// - MaterializedViews: /projects/{project}/instances/{instance}/materializedViews[/{materializedView}]
func bigtablePathBuilder(ctx base.PathContext) string {
	// For instances (top-level resources)
	if ctx.ResourceType == "instances" {
		path := fmt.Sprintf("/projects/%s/instances", ctx.Project)
		if ctx.ResourceName != "" {
			path += "/" + ctx.ResourceName
		}
		return path
	}

	// For backups (three-level hierarchy: instance > cluster > backup)
	// CustomSegments[0] should contain the cluster name
	if ctx.ResourceType == "backups" && ctx.ParentResource != "" && len(ctx.CustomSegments) > 0 {
		clusterName := ctx.CustomSegments[0]
		path := fmt.Sprintf("/projects/%s/instances/%s/clusters/%s/backups",
			ctx.Project, ctx.ParentResource, clusterName)
		if ctx.ResourceName != "" {
			path += "/" + ctx.ResourceName
		}
		return path
	}

	// For clusters and tables (nested under instances)
	if ctx.ParentResource != "" {
		path := fmt.Sprintf("/projects/%s/instances/%s/%s",
			ctx.Project, ctx.ParentResource, ctx.ResourceType)
		if ctx.ResourceName != "" {
			path += "/" + ctx.ResourceName
		}
		return path
	}

	// Fallback for other resource types
	path := fmt.Sprintf("/projects/%s/%s", ctx.Project, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractBigtableOperationID extracts operation name from Bigtable API response
// Bigtable returns LRO (Long Running Operation) with a "name" field
func extractBigtableOperationID(response map[string]interface{}) string {
	// Bigtable returns operation name in the "name" field
	// Format: "projects/{project}/instances/{instance}/operations/{operation-id}"
	// or: "projects/{project}/operations/{operation-id}"
	if name, ok := response["name"].(string); ok {
		// Check if it's an operation path (contains "/operations/")
		if strings.Contains(name, "/operations/") {
			return name
		}
	}
	return ""
}

// buildBigtableOperationURL constructs the URL to check operation status
// Bigtable operation URLs are the full operation name path
func buildBigtableOperationURL(ctx base.PathContext, operationID string) string {
	// operationID is already the full path
	// Format: "projects/{project}/instances/{instance}/operations/{operation-id}"
	// or: "projects/{project}/operations/{operation-id}"
	return operationID
}

// extractBigtableNativeID extracts the native ID (full path) from Bigtable API response
func extractBigtableNativeID(response map[string]interface{}, ctx base.PathContext) string {
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
			if ctx.ParentResource != "" {
				// For backups (three-level hierarchy)
				if ctx.ResourceType == "backups" && len(ctx.CustomSegments) > 0 {
					clusterName := ctx.CustomSegments[0]
					return fmt.Sprintf("projects/%s/instances/%s/clusters/%s/%s/%s",
						ctx.Project, ctx.ParentResource, clusterName, ctx.ResourceType, ctx.ResourceName)
				}
				// Nested resource (cluster or table)
				return fmt.Sprintf("projects/%s/instances/%s/%s/%s",
					ctx.Project, ctx.ParentResource, ctx.ResourceType, ctx.ResourceName)
			}
			// Top-level resource (instance)
			return fmt.Sprintf("projects/%s/%s/%s",
				ctx.Project, ctx.ResourceType, ctx.ResourceName)
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

// extractPathFromURL extracts the resource path from a full Bigtable URL
// Example: "https://bigtableadmin.googleapis.com/v2/projects/my-project/instances/my-instance"
// Returns: "projects/my-project/instances/my-instance"
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

// parseBigtableNativeID parses a Bigtable full-path native ID into PathContext
// Examples:
// - "projects/my-project/instances/my-instance"
// - "projects/my-project/instances/my-instance/clusters/my-cluster"
// - "projects/my-project/instances/my-instance/tables/my-table"
// - "projects/my-project/instances/my-instance/clusters/my-cluster/backups/my-backup"
// - "projects/my-project/instances/my-instance/materializedViews/my-view"
func parseBigtableNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 4 {
		return base.PathContext{}, fmt.Errorf("invalid bigtable native ID: %s", nativeID)
	}

	ctx := base.PathContext{}
	var instanceName string
	var clusterName string

	// Parse: projects/{project}/instances/{instance}[/resource-type/name][/resource-type/name]
	for i := 0; i < len(parts); i++ {
		switch parts[i] {
		case "projects":
			if i+1 < len(parts) {
				ctx.Project = parts[i+1]
				i++
			}
		case "instances":
			if i+1 < len(parts) {
				instanceName = parts[i+1]
				// Check if this is the final resource or a parent
				if i+2 >= len(parts) {
					// This is a standalone instance resource
					ctx.ResourceType = "instances"
					ctx.ResourceName = instanceName
				} else {
					// This is a parent for nested resources
					ctx.ParentResource = instanceName
				}
				i++
			}
		case "clusters":
			if i+1 < len(parts) {
				clusterName = parts[i+1]
				// Check if there's more (e.g., /backups/backup-name)
				if i+2 >= len(parts) || parts[i+2] != "backups" {
					// This is a standalone cluster resource
					ctx.ResourceType = "clusters"
					ctx.ResourceName = clusterName
					ctx.ParentResource = instanceName
				} else {
					// This cluster is a parent for backups
					// Store cluster name in CustomSegments for backup path building
					ctx.CustomSegments = []string{clusterName}
				}
				i++
			}
		case "tables":
			// Tables are directly under instances
			if i+1 < len(parts) {
				ctx.ResourceType = "tables"
				ctx.ResourceName = parts[i+1]
				ctx.ParentResource = instanceName
				i++
			}
		case "backups":
			// Backups are under clusters
			if i+1 < len(parts) {
				ctx.ResourceType = "backups"
				ctx.ResourceName = parts[i+1]
				ctx.ParentResource = instanceName
				// Cluster name should already be in CustomSegments
				if len(ctx.CustomSegments) == 0 && clusterName != "" {
					ctx.CustomSegments = []string{clusterName}
				}
				i++
			}
		default:
			// Every other Bigtable collection - materializedViews, appProfiles,
			// logicalViews, authorizedViews - sits directly under its instance
			// and needs no special handling. This used to be a per-collection
			// case, which meant an unlisted collection parsed to an empty
			// resource type and read nothing, silently. Only "projects" and the
			// cluster-scoped backups above are genuinely special.
			if i > 0 && i+1 < len(parts) && parts[i-1] == instanceName {
				ctx.ResourceType = parts[i]
				ctx.ResourceName = parts[i+1]
				ctx.ParentResource = instanceName
				i++
			}
		}
	}

	return ctx, nil
}

// checkBigtableOperationStatus checks if a Bigtable LRO operation is complete
func checkBigtableOperationStatus(operationResponse map[string]interface{}) (done bool, err error) {
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
