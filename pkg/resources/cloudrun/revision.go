// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package cloudrun

import (
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// revisionResponseTransformer transforms the API response for a Cloud Run revision
func revisionResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	props := make(map[string]interface{})

	// Basic fields - normalize name to short form
	if name, ok := apiResponse["name"].(string); ok {
		props["name"] = base.ExtractLastSegment(name)
		// Extract parent service/workerPool name from the full path
		// Format: projects/{p}/locations/{l}/services/{svc}/revisions/{rev}
		props["service"] = extractParentName(name)
	}
	if uid, ok := apiResponse["uid"].(string); ok {
		props["uid"] = uid
	}

	// Labels
	if labels, ok := apiResponse["labels"].(map[string]interface{}); ok {
		props["labels"] = labels
	}

	// Annotations
	if annotations, ok := apiResponse["annotations"].(map[string]interface{}); ok {
		props["annotations"] = annotations
	}

	// Scaling
	if scaling, ok := apiResponse["scaling"].(map[string]interface{}); ok {
		props["scaling"] = scaling
	}

	// Containers
	if containers, ok := apiResponse["containers"].([]interface{}); ok {
		props["containers"] = filterContainers(containers)
	}

	// Volumes
	if volumes, ok := apiResponse["volumes"].([]interface{}); ok {
		props["volumes"] = volumes
	}

	// Service account
	if sa, ok := apiResponse["serviceAccount"].(string); ok {
		props["serviceAccount"] = sa
	}

	// Add location/region from context
	if ctx.Location != "" {
		props["location"] = ctx.Location
	}
	props["project"] = ctx.Project
	if ctx.Region != "" {
		props["region"] = ctx.Region
	} else if ctx.Location != "" {
		props["region"] = ctx.Location
	}

	return props
}

// extractParentName extracts the parent resource name from a nested Cloud Run path
// Example: "projects/p/locations/l/services/my-svc/revisions/rev-1" → "my-svc"
func extractParentName(fullPath string) string {
	parts := strings.Split(fullPath, "/")
	// For 8-segment paths: projects/{p}/locations/{l}/{parentType}/{parentName}/{childType}/{childName}
	// Parent name is at index 5
	if len(parts) >= 8 {
		return parts[5]
	}
	return ""
}
