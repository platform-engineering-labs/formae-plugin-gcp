// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package gkehub

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// featureBodyBuilder builds the request body for creating a GKE Hub feature
func featureBodyBuilder(props map[string]interface{}) (map[string]interface{}, error) {
	body := make(map[string]interface{})

	if labels, ok := props["labels"].(map[string]interface{}); ok {
		body["labels"] = labels
	}

	if membershipSpecs, ok := props["membershipSpecs"].(map[string]interface{}); ok {
		body["membershipSpecs"] = membershipSpecs
	}

	if fleetDefault, ok := props["fleetDefaultMemberConfig"].(map[string]interface{}); ok {
		body["fleetDefaultMemberConfig"] = fleetDefault
	}

	return body, nil
}

// featureResponseTransformer passes through the API response
func featureResponseTransformer(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range apiResponse {
		result[k] = v
	}

	if name, ok := result["name"].(string); ok {
		result["name"] = base.ExtractLastSegment(name)
	}

	return result
}
