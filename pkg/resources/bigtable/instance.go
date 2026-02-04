// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigtable

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
)

// instanceBodyBuilderWithContext builds the request body for creating a Bigtable instance
// Note: The Bigtable API has a special structure where the instance object and clusters
// are at the same level in the request, NOT nested.
func instanceBodyBuilderWithContext(props map[string]interface{}, project string) (map[string]interface{}, error) {
	instanceBody := make(map[string]interface{})

	// Display name
	if displayName := utils.GetString(props, "displayName"); displayName != "" {
		instanceBody["displayName"] = displayName
	}

	// Instance type
	if instanceType := utils.GetString(props, "type"); instanceType != "" {
		instanceBody["type"] = instanceType
	} else {
		instanceBody["type"] = "PRODUCTION" // default
	}

	// Labels
	if labels := getStringMap(props, "labels"); labels != nil {
		instanceBody["labels"] = labels
	}

	// Build the complete body structure
	// For Bigtable, clusters must be at the root level, not inside "instance"
	body := map[string]interface{}{
		"instance": instanceBody,
	}

	// Determine instance type for cluster configuration
	instanceType := utils.GetString(props, "type")
	if instanceType == "" {
		instanceType = "PRODUCTION"
	}

	// Clusters (can be created separately or with instance)
	if clusters := getClustersMap(props, project, instanceType); clusters != nil {
		body["clusters"] = clusters
	}

	return body, nil
}

// getClustersMap builds the clusters map for instance creation
func getClustersMap(props map[string]interface{}, project string, instanceType string) map[string]interface{} {
	if val, ok := props["clusters"]; ok {
		if clustersMap, ok := val.(map[string]interface{}); ok {
			result := make(map[string]interface{})
			for clusterID, clusterVal := range clustersMap {
				if clusterProps, ok := clusterVal.(map[string]interface{}); ok {
					cluster := make(map[string]interface{})

					// Location - Must be in format projects/{project}/locations/{zone}
					if location := utils.GetString(clusterProps, "location"); location != "" {
						// If location is not already in full path format, convert it
						if !strings.HasPrefix(location, "projects/") {
							location = fmt.Sprintf("projects/%s/locations/%s", project, location)
						}
						cluster["location"] = location
					}

					// Serve nodes (only for PRODUCTION instances)
					// DEVELOPMENT instances must not have serveNodes specified
					if instanceType == "PRODUCTION" {
						if serveNodes := utils.GetInt32(clusterProps, "serveNodes"); serveNodes > 0 {
							cluster["serveNodes"] = serveNodes
						}
					}

					// Default storage type
					if storageType := utils.GetString(clusterProps, "defaultStorageType"); storageType != "" {
						cluster["defaultStorageType"] = storageType
					} else {
						cluster["defaultStorageType"] = "SSD" // default
					}

					// Encryption config
					if encConfig := utils.GetObject(clusterProps, "encryptionConfig"); encConfig != nil {
						encryptionMap := make(map[string]interface{})
						if kmsKeyName := utils.GetString(encConfig, "kmsKeyName"); kmsKeyName != "" {
							encryptionMap["kmsKeyName"] = kmsKeyName
						}
						cluster["encryptionConfig"] = encryptionMap
					}

					result[clusterID] = cluster
				}
			}
			return result
		}
	}
	return nil
}

// instanceResponseTransformer transforms the API response into a normalized format
func instanceResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	apiResponse["project"] = ctx.Project
	// The API returns name as full path (projects/.../instances/...), normalize to short name
	if name := utils.GetString(apiResponse, "name"); name != "" {
		apiResponse["name"] = base.ExtractLastSegment(name)
	}
	apiResponse["instanceName"] = apiResponse["name"]
	return apiResponse
}

// Helper function
func getStringMap(m map[string]interface{}, key string) map[string]interface{} {
	if val, ok := m[key]; ok {
		if strMap, ok := val.(map[string]interface{}); ok {
			return strMap
		}
	}
	return nil
}
