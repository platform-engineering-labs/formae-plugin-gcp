// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dns

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

const (
	ManagedZoneResourceType = "GCP::DNS::ManagedZone"
	PolicyResourceType      = "GCP::DNS::Policy"
)

var dnsRegistry *base.ResourceRegistry

func init() {
	dnsRegistry = base.NewResourceRegistry(DNSAPI, DNSOperations, DNSNativeID)

	// ManagedZone fits the generic engine: create is a plain POST to the
	// collection with the name in the body; Read/Delete/List operate on the
	// full resource path. No custom provisioner needed.
	err := dnsRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: ManagedZoneResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "managedZones",
				SupportsUpdate: false, // ponytail: description/labels are patchable; defer until verified
			},
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// A policy fits the same engine: the name travels in the body, and
			// the path is the generic /projects/{p}/policies[/{name}]. Cloud DNS
			// patches a policy with the whole object rather than a field mask.
			ResourceType: PolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "policies",
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPatch,
				ListItemsKey:   "policies",
			},
			RequestTransformer:  base.RequestTransformerFunc(policyRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(policyResponseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}
}
