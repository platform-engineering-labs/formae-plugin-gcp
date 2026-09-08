// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// firewallResponseTransformer normalizes the Firewall API response by ensuring
// empty ports arrays are preserved in allowed/denied rules.
//
// `network` is deliberately left as the provider returned it. The Compute API
// is lenient on input and canonical on output: its discovery document lists
// three accepted forms for Firewall.network (full URL, "projects/{p}/global/
// networks/{n}", "global/networks/default"), but Network.selfLink is
// "[Output Only] Server-defined URL for the resource" and a read always answers
// with the full URL whichever form was written - GCP's own default-allow-icmp
// reads back as a full URL (verified live 2026-09-08).
//
// So the full self-link is the only form that survives a round trip, which is
// why Subnetwork, Router and Instance all keep it (PLA-265). This transformer
// used to strip the prefix, which left a firewall declaring the documented
// `network = net.res.selfLink` idiom diffing a short stored path against a full
// desired URL - a spurious replace on every reconcile, since network is
// createOnly (PLA-251).
func firewallResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy all fields
	for k, v := range apiResponse {
		result[k] = v
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
