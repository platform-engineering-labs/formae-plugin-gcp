// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package sql

import "strconv"

// databaseInstanceResponseTransformer transforms Cloud SQL API response to Formae format
func databaseInstanceResponseTransformer(apiResponse map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy all fields from API response
	for k, v := range apiResponse {
		result[k] = v
	}

	// Convert settings.dataDiskSizeGb from string to int
	if settings, ok := result["settings"].(map[string]interface{}); ok {
		if dataDiskSizeGb, ok := settings["dataDiskSizeGb"].(string); ok {
			if size, err := strconv.ParseInt(dataDiskSizeGb, 10, 64); err == nil {
				settings["dataDiskSizeGb"] = size
			}
		}
	}

	return result
}
