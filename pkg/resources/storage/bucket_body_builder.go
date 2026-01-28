// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package storage

import "strings"

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
	// Location comes back as uppercase from GCP, convert to lowercase
	if location, ok := apiResponse["location"].(string); ok {
		apiResponse["location"] = strings.ToLower(location)
	}

	return apiResponse
}
