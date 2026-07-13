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

	// Normalize ipConfiguration.privateNetwork to the full compute selfLink form.
	// The forma declares this from a network resolvable's .selfLink (a full
	// "https://www.googleapis.com/compute/v1/projects/..." URL), but Cloud SQL
	// stores/echoes the bare "projects/..." path, which would otherwise drift.
	// Expand the short path back to the selfLink so it round-trips.
	if ipc, ok := settings["ipConfiguration"].(map[string]interface{}); ok {
		if pn, ok := ipc["privateNetwork"].(string); ok {
			if strings.HasPrefix(pn, "projects/") {
				ipc["privateNetwork"] = "https://www.googleapis.com/compute/v1/" + pn
			}
		}
	}

	return apiResponse
}
