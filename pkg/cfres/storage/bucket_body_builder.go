// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package storage

// bucketBodyBuilder transforms Formae properties to GCP Storage API format
func bucketBodyBuilder(props map[string]interface{}) (map[string]interface{}, error) {
	body := make(map[string]interface{})

	// Direct passthrough fields
	directFields := []string{"name", "location", "storageClass", "labels", "versioning", "cors", "website", "encryption", "logging", "retentionPolicy", "rpo", "iamConfiguration", "hierarchicalNamespace"}
	for _, field := range directFields {
		if val, ok := props[field]; ok {
			body[field] = val
		}
	}

	// Transform lifecycleRule -> lifecycle
	if lifecycleRules, ok := props["lifecycleRule"].([]interface{}); ok {
		body["lifecycle"] = map[string]interface{}{
			"rule": lifecycleRules,
		}
	}

	// Transform IAM configuration fields
	if ubla, ok := props["uniformBucketLevelAccess"].(bool); ok {
		if body["iamConfiguration"] == nil {
			body["iamConfiguration"] = make(map[string]interface{})
		}
		iamConfig := body["iamConfiguration"].(map[string]interface{})
		iamConfig["uniformBucketLevelAccess"] = map[string]interface{}{
			"enabled": ubla,
		}
	}

	if pap, ok := props["publicAccessPrevention"].(string); ok {
		if body["iamConfiguration"] == nil {
			body["iamConfiguration"] = make(map[string]interface{})
		}
		iamConfig := body["iamConfiguration"].(map[string]interface{})
		iamConfig["publicAccessPrevention"] = pap
	}

	// Transform requesterPays -> billing
	if requesterPays, ok := props["requesterPays"].(bool); ok {
		body["billing"] = map[string]interface{}{
			"requesterPays": requesterPays,
		}
	}

	return body, nil
}

// bucketResponseTransformer transforms GCP Storage API response to Formae format
func bucketResponseTransformer(apiResponse map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy all fields
	for k, v := range apiResponse {
		result[k] = v
	}

	// Transform lifecycle.rule -> lifecycleRule
	if lifecycle, ok := apiResponse["lifecycle"].(map[string]interface{}); ok {
		if rules, ok := lifecycle["rule"].([]interface{}); ok {
			result["lifecycleRule"] = rules
		}
	}

	// Transform iamConfiguration fields to top-level
	if iamConfig, ok := apiResponse["iamConfiguration"].(map[string]interface{}); ok {
		if ubla, ok := iamConfig["uniformBucketLevelAccess"].(map[string]interface{}); ok {
			if enabled, ok := ubla["enabled"].(bool); ok {
				result["uniformBucketLevelAccess"] = enabled
			}
		}
		if pap, ok := iamConfig["publicAccessPrevention"].(string); ok {
			result["publicAccessPrevention"] = pap
		}
	}

	// Transform billing.requesterPays to top-level
	if billing, ok := apiResponse["billing"].(map[string]interface{}); ok {
		if requesterPays, ok := billing["requesterPays"].(bool); ok {
			result["requesterPays"] = requesterPays
		}
	}

	return result
}
