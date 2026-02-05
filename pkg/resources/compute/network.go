// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// networkResponseTransformer normalizes the Network API response:
// 1. Filters out bgpBestPathSelectionMode from routingConfig (read-only field added by API)
func networkResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy all fields
	for k, v := range apiResponse {
		result[k] = v
	}

	// Filter out bgpBestPathSelectionMode from routingConfig
	// This is a read-only field that the API returns but is not part of the schema
	if routingConfig, ok := result["routingConfig"].(map[string]interface{}); ok {
		filteredConfig := make(map[string]interface{})
		for k, v := range routingConfig {
			// Skip bgpBestPathSelectionMode - it's a read-only field not in the schema
			if k != "bgpBestPathSelectionMode" {
				filteredConfig[k] = v
			}
		}
		result["routingConfig"] = filteredConfig
	}

	return result
}

// NetworkResponseTransformer is the response transformer for Network resources
var NetworkResponseTransformer = base.ResponseTransformerFunc(networkResponseTransformer)
