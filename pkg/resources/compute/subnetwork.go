// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// subnetworkResponseTransformer normalizes the Subnetwork API response:
// 1. Extracts region name from full URL
// 2. Strips the full API URL prefix from the network field
func subnetworkResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy all fields
	for k, v := range apiResponse {
		result[k] = v
	}

	// Extract region name from full URL
	if region, ok := result["region"].(string); ok && region != "" {
		result["region"] = base.ExtractLastSegment(region)
	}

	// Normalize network URL: strip the API prefix to get project-relative path
	if network, ok := result["network"].(string); ok && network != "" {
		result["network"] = strings.TrimPrefix(network, computeAPIPrefix)
	}

	return result
}

// SubnetworkResponseTransformer is the response transformer for Subnetwork resources
var SubnetworkResponseTransformer = base.ResponseTransformerFunc(subnetworkResponseTransformer)
