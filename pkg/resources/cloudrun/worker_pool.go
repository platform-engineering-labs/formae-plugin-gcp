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
		body["template"] = buildRevisionTemplate(templateProps)
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

	// Instance split
	if instanceSplit := getInstanceSplitArray(props); instanceSplit != nil {
		body["instanceSplit"] = instanceSplit
	}

	return body, nil
}

// getInstanceSplitArray builds the instance split array for worker pools
func getInstanceSplitArray(props map[string]interface{}) []map[string]interface{} {
	if val, ok := props["instanceSplit"]; ok {
		if arr, ok := val.([]interface{}); ok {
			result := make([]map[string]interface{}, 0, len(arr))
			for _, item := range arr {
				if obj, ok := item.(map[string]interface{}); ok {
					split := make(map[string]interface{})
					if percent := utils.GetInt32(obj, "percent"); percent > 0 {
						split["percent"] = percent
					}
					if revision := utils.GetString(obj, "revision"); revision != "" {
						split["type"] = "INSTANCE_SPLIT_ALLOCATION_TYPE_REVISION"
						split["revision"] = revision
					} else {
						split["type"] = "INSTANCE_SPLIT_ALLOCATION_TYPE_LATEST"
					}
					result = append(result, split)
				}
			}
			return result
		}
	}
	return nil
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
