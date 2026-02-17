// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package cloudrun

import (
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// taskResponseTransformer transforms the API response for a Cloud Run task
func taskResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	props := make(map[string]interface{})

	// Basic fields - normalize name to short form
	if name, ok := apiResponse["name"].(string); ok {
		props["name"] = base.ExtractLastSegment(name)

		// Extract parent execution and grandparent job from the full path
		// Format: projects/{p}/locations/{l}/jobs/{job}/executions/{exec}/tasks/{task}
		parts := strings.Split(name, "/")
		if len(parts) >= 10 {
			props["job"] = parts[5]
			props["execution"] = parts[7]
		}
	}
	if uid, ok := apiResponse["uid"].(string); ok {
		props["uid"] = uid
	}

	// Index
	if index, ok := apiResponse["index"].(float64); ok {
		props["index"] = int(index)
	}

	// Timing fields
	if startTime, ok := apiResponse["startTime"].(string); ok {
		props["startTime"] = startTime
	}
	if completionTime, ok := apiResponse["completionTime"].(string); ok {
		props["completionTime"] = completionTime
	}

	// Log URI
	if logUri, ok := apiResponse["logUri"].(string); ok {
		props["logUri"] = logUri
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
