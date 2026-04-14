// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package gkehub

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
)

// membershipBodyBuilder builds the request body for creating a GKE Hub membership
func membershipBodyBuilder(props map[string]interface{}) (map[string]interface{}, error) {
	body := make(map[string]interface{})

	if description := utils.GetString(props, "description"); description != "" {
		body["description"] = description
	}

	if labels, ok := props["labels"].(map[string]interface{}); ok {
		body["labels"] = labels
	}

	if endpointProps := utils.GetObject(props, "endpoint"); endpointProps != nil {
		endpoint := make(map[string]interface{})
		if gkeCluster := utils.GetObject(endpointProps, "gkeCluster"); gkeCluster != nil {
			gc := make(map[string]interface{})
			if resourceLink := utils.GetString(gkeCluster, "resourceLink"); resourceLink != "" {
				gc["resourceLink"] = resourceLink
			}
			if len(gc) > 0 {
				endpoint["gkeCluster"] = gc
			}
		}
		if len(endpoint) > 0 {
			body["endpoint"] = endpoint
		}
	}

	if authorityProps := utils.GetObject(props, "authority"); authorityProps != nil {
		authority := make(map[string]interface{})
		if issuer := utils.GetString(authorityProps, "issuer"); issuer != "" {
			authority["issuer"] = issuer
		}
		if len(authority) > 0 {
			body["authority"] = authority
		}
	}

	return body, nil
}

// membershipResponseTransformer passes through the API response as-is
func membershipResponseTransformer(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range apiResponse {
		result[k] = v
	}

	// Normalize name to short form if it's a full path
	if name, ok := result["name"].(string); ok {
		result["name"] = base.ExtractLastSegment(name)
	}

	return result
}
