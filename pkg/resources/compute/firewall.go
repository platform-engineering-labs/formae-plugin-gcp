// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

const computeAPIPrefix = "https://www.googleapis.com/compute/v1/"

// firewallResponseTransformer normalizes the Firewall API response:
// 1. Strips the full API URL prefix from the network field
// 2. Ensures empty ports arrays are preserved in allowed/denied rules
func firewallResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy all fields
	for k, v := range apiResponse {
		result[k] = v
	}

	// Normalize network URL: strip the API prefix to get project-relative path
	// e.g., "https://www.googleapis.com/compute/v1/projects/my-project/global/networks/my-network"
	//    -> "projects/my-project/global/networks/my-network"
	if network, ok := result["network"].(string); ok && network != "" {
		result["network"] = strings.TrimPrefix(network, computeAPIPrefix)
	}

	// Normalize allowed rules: ensure empty ports arrays are preserved
	if allowed, ok := result["allowed"].([]interface{}); ok {
		result["allowed"] = normalizeFirewallRules(allowed)
	}

	// Normalize denied rules: ensure empty ports arrays are preserved
	if denied, ok := result["denied"].([]interface{}); ok {
		result["denied"] = normalizeFirewallRules(denied)
	}

	return result
}

// normalizeFirewallRules ensures each rule has a ports field (empty array if not present)
func normalizeFirewallRules(rules []interface{}) []interface{} {
	normalized := make([]interface{}, len(rules))

	for i, rule := range rules {
		if ruleMap, ok := rule.(map[string]interface{}); ok {
			normalizedRule := make(map[string]interface{})

			// Copy all fields
			for k, v := range ruleMap {
				normalizedRule[k] = v
			}

			// Ensure ports field exists (even if empty)
			if _, hasPorts := normalizedRule["ports"]; !hasPorts {
				normalizedRule["ports"] = []interface{}{}
			}

			normalized[i] = normalizedRule
		} else {
			normalized[i] = rule
		}
	}

	return normalized
}

// FirewallResponseTransformer is the response transformer for Firewall resources
var FirewallResponseTransformer = base.ResponseTransformerFunc(firewallResponseTransformer)
