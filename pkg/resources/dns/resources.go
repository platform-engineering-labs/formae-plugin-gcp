// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dns

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

const ManagedZoneResourceType = "GCP::DNS::ManagedZone"

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
	})
	if err != nil {
		panic(err)
	}
}
