// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// subnetworkResponseTransformer normalizes the Subnetwork API response:
// 1. Extracts region name from full URL
//
// The `network` field is left as the provider's full self-link URL so it matches
// a resolvable network reference (`net.res.selfLink`) - the same URL the API
// returns. Rewriting it to a "projects/..." path would break that match and make
// an unchanged forma re-apply plan a spurious replace (see PLA-265). This mirrors
// how attached-disk `source` is handled on the Instance resource.
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

	return result
}

// SubnetworkResponseTransformer is the response transformer for Subnetwork resources
var SubnetworkResponseTransformer = base.ResponseTransformerFunc(subnetworkResponseTransformer)
