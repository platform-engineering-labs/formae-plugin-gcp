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

// listParentWildcards names the parent path a discovery list substitutes for
// each nested collection. Cloud Run accepts "-" in the parent position, probed
// live 2026-09-08 against project development-477117:
//
//	GET .../locations/europe-central2/services/-/revisions   -> 200, and it
//	    returned a real revision under a real service, so the wildcard
//	    enumerates rather than merely being tolerated.
//	GET .../locations/us-central1/jobs/-/executions          -> 200
//	GET .../locations/us-central1/jobs/-/executions/-/tasks   -> 200
//
// A task is two collections deep, hence the two wildcards in one string:
// PathContext carries a single parent level, and this is the whole parent path.
//
// There is deliberately no entry for a top-level type. Only one wildcard is
// allowed per path - locations/- together with services/- answers 400 "Request
// contains an invalid argument" - and locations/- is not uniformly available
// anyway: it answers 200 for services, 400 for jobs and 501 for workerPools.
var listParentWildcards = map[string]string{
	"revisions":  "services/-",
	"executions": "jobs/-",
	"tasks":      "jobs/-/executions/-",
}

// cloudRunPathBuilder builds Cloud Run API paths with location-based scoping
// Cloud Run v2 API format: /projects/{project}/locations/{location}/{resourceType}[/{name}]
// Nested resources: /projects/{project}/locations/{location}/{parentType}/{parentName}/{resourceType}[/{name}]
// Special case for Create: adds query parameter ?serviceId={name} or ?jobId={name}
func cloudRunPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s", ctx.Project, cloudRunLocation(ctx))

	// For nested resources, include parent path segments
	if ctx.ParentType != "" && ctx.ParentResource != "" {
		path += fmt.Sprintf("/%s/%s", ctx.ParentType, ctx.ParentResource)
	} else if ctx.IsList {
		// Discovery lists with no properties at all, so a nested type has no
		// parent to name and the branch above cannot fire. Without the
		// wildcard the path drops the parent collection entirely and asks for
		// e.g. /locations/{l}/revisions, which does not exist.
		if wildcard, ok := listParentWildcards[ctx.ResourceType]; ok {
			path += "/" + wildcard
		}
	}

	path += "/" + ctx.ResourceType

	// For specific resource operations (Read, Delete, Status), append the resource name
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}

	return path
}

// cloudRunLocation resolves the location segment, falling back to the target's
// region.
//
// Cloud Run v2 has no zones and no second concept for a region to name: a
// location *is* a region. base deliberately does not derive one from the other
// ("Container/CloudRun use location (no Region fallback)" in
// base_resource_helpers.go), so a target that sets region and leaves location
// unset - the shape /formae:connect writes - interpolated the empty string and
// asked for "locations//services", which the API answers 400 on every
// discovery cycle. Resolving it here rather than in base keeps GKE out of it,
// where a location may legitimately be a zone.
func cloudRunLocation(ctx base.PathContext) string {
	if ctx.Location != "" {
		return ctx.Location
	}
	return ctx.Region
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
			// A hardcoded default here writes a native ID naming a region the
			// resource is not in, and unlike a bad list path it fails silently:
			// no 404, just a wrong stored id.
			return fmt.Sprintf("projects/%s/locations/%s/%s/%s",
				ctx.Project, cloudRunLocation(ctx), ctx.ResourceType, ctx.ResourceName)
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
// Handles both top-level and nested resource paths:
//   - 6 segments: projects/{project}/locations/{location}/{resourceType}/{name}
//   - 8 segments: projects/{project}/locations/{location}/{parentType}/{parentName}/{resourceType}/{name}
func parseCloudRunNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 6 {
		return base.PathContext{}, fmt.Errorf("invalid cloud run native ID: %s", nativeID)
	}

	ctx := base.PathContext{}

	// Parse known key/value segments first
	// Collect remaining segments as resource type pairs
	var resourceSegments []string
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
			// Collect remaining resource type/name pairs
			resourceSegments = append(resourceSegments, parts[i])
		}
	}

	// With resource segments, last pair is the resource, earlier pairs are parents
	// e.g., ["services", "my-svc", "revisions", "my-rev"] → parent=services/my-svc, resource=revisions/my-rev
	// e.g., ["jobs", "j", "executions", "e", "tasks", "t"] → parent=jobs, parentResource=j/executions/e, resource=tasks/t
	if len(resourceSegments) >= 6 {
		// Multi-level nesting (e.g., jobs/j/executions/e/tasks/t)
		ctx.ParentType = resourceSegments[0]
		ctx.ParentResource = strings.Join(resourceSegments[1:len(resourceSegments)-2], "/")
		ctx.ResourceType = resourceSegments[len(resourceSegments)-2]
		ctx.ResourceName = resourceSegments[len(resourceSegments)-1]
	} else if len(resourceSegments) >= 4 {
		// Nested resource: first pair is parent, last pair is resource
		ctx.ParentType = resourceSegments[0]
		ctx.ParentResource = resourceSegments[1]
		ctx.ResourceType = resourceSegments[len(resourceSegments)-2]
		ctx.ResourceName = resourceSegments[len(resourceSegments)-1]
	} else if len(resourceSegments) >= 2 {
		// Top-level resource
		ctx.ResourceType = resourceSegments[0]
		ctx.ResourceName = resourceSegments[1]
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
