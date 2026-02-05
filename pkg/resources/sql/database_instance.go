// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package sql

import "strconv"

// databaseInstanceResponseTransformer transforms Cloud SQL API response to Formae format
func databaseInstanceResponseTransformer(apiResponse map[string]interface{}) map[string]interface{} {
	// Convert settings.dataDiskSizeGb from string to int if needed
	if settings, ok := apiResponse["settings"].(map[string]interface{}); ok {
		if dataDiskSizeGb, ok := settings["dataDiskSizeGb"].(string); ok {
			if size, err := strconv.ParseInt(dataDiskSizeGb, 10, 64); err == nil {
				settings["dataDiskSizeGb"] = size
			}
		}
	}

	return apiResponse
}
