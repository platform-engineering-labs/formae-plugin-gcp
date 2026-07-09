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

	// Drop provider-assigned "noise": fields GCP always populates that no forma
	// declares, so they otherwise read back as perpetual drift.
	// Mirrors of separately-managed resources (not in the Network schema);
	// reporting them here drifts the network whenever a subnet (Subnetwork) or
	// PSA peering (Connection) attaches. Schema fields GCP defaults
	// (routingConfig, mtu, networkFirewallPolicyEnforcementOrder) carry
	// hasProviderDefault instead of being stripped.
	for _, k := range []string{
		"subnetworks",
		"peerings",
	} {
		delete(result, k)
	}

	return result
}

// NetworkResponseTransformer is the response transformer for Network resources
var NetworkResponseTransformer = base.ResponseTransformerFunc(networkResponseTransformer)
