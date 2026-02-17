// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package cloudrun

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// executionResponseTransformer transforms the API response for a Cloud Run execution
func executionResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	props := make(map[string]interface{})

	// Basic fields - normalize name to short form
	if name, ok := apiResponse["name"].(string); ok {
		props["name"] = base.ExtractLastSegment(name)
		// Extract parent job name from the full path
		// Format: projects/{p}/locations/{l}/jobs/{job}/executions/{exec}
		props["job"] = extractParentName(name)
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

	// Task count
	if taskCount, ok := apiResponse["taskCount"].(float64); ok {
		props["taskCount"] = int(taskCount)
	}

	// Parallelism
	if parallelism, ok := apiResponse["parallelism"].(float64); ok {
		props["parallelism"] = int(parallelism)
	}

	// Template
	if template, ok := apiResponse["template"].(map[string]interface{}); ok {
		props["template"] = template
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
