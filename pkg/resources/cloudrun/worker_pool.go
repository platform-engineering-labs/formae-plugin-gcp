// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package cloudrun

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
)

// workerPoolBodyBuilder builds the request body for creating a Cloud Run worker pool
func workerPoolBodyBuilder(props map[string]interface{}) (map[string]interface{}, error) {
	body := make(map[string]interface{})

	// Description
	if desc := utils.GetString(props, "description"); desc != "" {
		body["description"] = desc
	}

	// Labels
	if labels := getStringMap(props, "labels"); labels != nil {
		body["labels"] = labels
	}

	// Annotations
	if annotations := getStringMap(props, "annotations"); annotations != nil {
		body["annotations"] = annotations
	}

	// Template (revision template)
	if templateProps := utils.GetObject(props, "template"); templateProps != nil {
		body["template"] = buildWorkerPoolRevisionTemplate(templateProps)
	}

	// Top-level scaling
	if scaling := utils.GetObject(props, "scaling"); scaling != nil {
		scalingMap := make(map[string]interface{})
		if minInstanceCount := utils.GetInt32(scaling, "minInstanceCount"); minInstanceCount > 0 {
			scalingMap["minInstanceCount"] = minInstanceCount
		}
		if maxInstanceCount := utils.GetInt32(scaling, "maxInstanceCount"); maxInstanceCount > 0 {
			scalingMap["maxInstanceCount"] = maxInstanceCount
		}
		if len(scalingMap) > 0 {
			body["scaling"] = scalingMap
		}
	}

	// Launch stage
	if launchStage := utils.GetString(props, "launchStage"); launchStage != "" {
		body["launchStage"] = launchStage
	}

	return body, nil
}

// buildWorkerPoolRevisionTemplate wraps buildRevisionTemplate and strips fields
// that are not supported in the WorkerPool revision template (e.g. scaling).
func buildWorkerPoolRevisionTemplate(templateProps map[string]interface{}) map[string]interface{} {
	template := buildRevisionTemplate(templateProps)
	delete(template, "scaling")
	return template
}

// workerPoolResponseTransformer transforms the API response into a normalized format
func workerPoolResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	props := make(map[string]interface{})

	// Basic fields - normalize name to short form
	if name, ok := apiResponse["name"].(string); ok {
		props["name"] = base.ExtractLastSegment(name)
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

	// Template - filter out API-added defaults not in schema
	if template, ok := apiResponse["template"].(map[string]interface{}); ok {
		props["template"] = filterTemplate(template)
	}

	// Top-level scaling
	if scaling, ok := apiResponse["scaling"].(map[string]interface{}); ok {
		props["scaling"] = scaling
	}

	// Instance split
	if instanceSplit, ok := apiResponse["instanceSplit"].([]interface{}); ok {
		props["instanceSplit"] = instanceSplit
	}

	// Add location if not present (from context)
	if _, ok := props["location"]; !ok && ctx.Location != "" {
		props["location"] = ctx.Location
	}

	props["project"] = ctx.Project
	// Cloud Run uses location, but PKL schema expects "region" - use Location as Region
	if ctx.Region != "" {
		props["region"] = ctx.Region
	} else if ctx.Location != "" {
		props["region"] = ctx.Location
	}

	return props
}
