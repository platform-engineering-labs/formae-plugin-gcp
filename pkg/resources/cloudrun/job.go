// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package cloudrun

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
)

// jobBodyBuilder builds the request body for creating a Cloud Run job
func jobBodyBuilder(props map[string]interface{}) (map[string]interface{}, error) {

	// Remove location from props (passed as URL parameter)
	if location := utils.GetString(props, "location"); location != "" {
		delete(props, "location")
	}

	// Remove name from props (passed as job_id URL parameter)
	if name := utils.GetString(props, "name"); name != "" {
		delete(props, "name")
	}

	return props, nil
}

// jobResponseTransformer transforms the API response into a normalized format
func jobResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	props := make(map[string]interface{})

	// Basic fields
	if name, ok := apiResponse["name"].(string); ok {
		props["name"] = name
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

	// Template
	if template, ok := apiResponse["template"].(map[string]interface{}); ok {
		props["template"] = template
	}

	// Latest created execution
	if latestExecution, ok := apiResponse["latestCreatedExecution"].(map[string]interface{}); ok {
		if execName, ok := latestExecution["name"].(string); ok {
			props["latestCreatedExecution"] = execName
		}
	}

	// Add location if not present (from context)
	if _, ok := props["location"]; !ok && ctx.Location != "" {
		props["location"] = ctx.Location
	}

	return props
}
