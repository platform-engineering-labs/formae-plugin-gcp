// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package container

// clusterResponseTransformer transforms GKE Container API response to Formae format
func clusterResponseTransformer(apiResponse map[string]interface{}, location string) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy all fields from API response
	for k, v := range apiResponse {
		result[k] = v
	}

	// Extract cluster name from selfLink if not present
	// selfLink format: https://container.googleapis.com/v1/projects/{project}/locations/{location}/clusters/{clusterName}
	if selfLink, ok := result["selfLink"].(string); ok && selfLink != "" {
		if components, err := ExtractClusterNameFromTargetLink(selfLink); err == nil {
			// Set name if not present (use cluster name from selfLink)
			// We add the clusterName field to have this information present in the cluster
			result["clusterName"] = components.ClusterName
		}
	}

	// Fallback: set location from parameter if still not present
	if _, ok := result["location"]; !ok {
		result["location"] = location
	}

	return result
}

// nodePoolResponseTransformer transforms GKE Container API response to Formae format for NodePools
func nodePoolResponseTransformer(apiResponse map[string]interface{}, location string) map[string]interface{} {
	result := make(map[string]interface{})

	for k, v := range apiResponse {
		result[k] = v
	}

	// Extract cluster name and location from selfLink if not already present
	// selfLink format: https://container.googleapis.com/v1/projects/{project}/locations/{location}/clusters/{cluster}/nodePools/{nodePool}
	if selfLink, ok := result["selfLink"].(string); ok && selfLink != "" {
		if components, err := ExtractNodePoolNameFromTargetLink(selfLink); err == nil {
			// Set cluster name if not present
			if _, ok := result["cluster"]; !ok {
				result["cluster"] = components.ClusterName
			}
			// Set location from selfLink if not present (more accurate than passed parameter)
			if _, ok := result["location"]; !ok {
				result["location"] = components.Location
			}
		}
	}

	return result
}

// Example: If you need custom transformation logic in the future, create a body builder:
//
// import "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
//
// func customBodyBuilder(props map[string]interface{}) (map[string]interface{}, error) {
//     // Perform custom transformations here
//     // For example, converting field names, restructuring nested objects, etc.
//
//     transformed := make(map[string]interface{})
//
//     // Custom logic...
//     if val := utils.GetString(props, "customField"); val != "" {
//         transformed["transformedField"] = val
//     }
//
//     return transformed, nil
// }
