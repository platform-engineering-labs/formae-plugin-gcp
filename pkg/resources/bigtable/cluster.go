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

// clusterBodyBuilder builds the request body for creating a Bigtable cluster
func clusterBodyBuilder(props map[string]interface{}, project string) (map[string]interface{}, error) {
	body := make(map[string]interface{})

	// Location (required for cluster creation)
	// Must be in format projects/{project}/locations/{zone}
	if location := utils.GetString(props, "location"); location != "" {
		// If location is not already in full path format, convert it
		if !strings.HasPrefix(location, "projects/") {
			location = fmt.Sprintf("projects/%s/locations/%s", project, location)
		}
		body["location"] = location
	}

	// Serve nodes (required for PRODUCTION instances)
	if serveNodes := utils.GetInt32(props, "serveNodes"); serveNodes > 0 {
		body["serveNodes"] = serveNodes
	}

	// Default storage type
	if storageType := utils.GetString(props, "defaultStorageType"); storageType != "" {
		body["defaultStorageType"] = storageType
	} else {
		body["defaultStorageType"] = "SSD" // default
	}

	// Encryption config
	if encConfig := utils.GetObject(props, "encryptionConfig"); encConfig != nil {
		encryptionMap := make(map[string]interface{})
		if kmsKeyName := utils.GetString(encConfig, "kmsKeyName"); kmsKeyName != "" {
			encryptionMap["kmsKeyName"] = kmsKeyName
		}
		body["encryptionConfig"] = encryptionMap
	}

	return body, nil
}

// clusterResponseTransformer transforms the API response into a normalized format
func clusterResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	//if serveNodes, ok := apiResponse["serveNodes"].(float64); ok {
	//	props["serveNodes"] = int32(serveNodes)
	//}

	apiResponse["project"] = ctx.Project

	return apiResponse
}
