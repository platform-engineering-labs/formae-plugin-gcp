// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package sql

import (
	"strconv"
	"strings"
)

// databaseInstanceResponseTransformer transforms Cloud SQL API response to Formae format
func databaseInstanceResponseTransformer(apiResponse map[string]interface{}) map[string]interface{} {
	settings, ok := apiResponse["settings"].(map[string]interface{})
	if !ok {
		return apiResponse
	}

	// Convert settings.dataDiskSizeGb from string to int if needed
	if dataDiskSizeGb, ok := settings["dataDiskSizeGb"].(string); ok {
		if size, err := strconv.ParseInt(dataDiskSizeGb, 10, 64); err == nil {
			settings["dataDiskSizeGb"] = size
		}
	}

	// Normalize ipConfiguration.privateNetwork to the "projects/..." path the
	// API accepts and that a network resolvable reference resolves to. Cloud SQL
	// echoes it back as a full "https://www.googleapis.com/compute/v1/projects/..."
	// URL, which would otherwise drift against the declared value.
	if ipc, ok := settings["ipConfiguration"].(map[string]interface{}); ok {
		if pn, ok := ipc["privateNetwork"].(string); ok {
			if i := strings.Index(pn, "projects/"); i >= 0 {
				ipc["privateNetwork"] = pn[i:]
			}
		}
	}

	return apiResponse
}
