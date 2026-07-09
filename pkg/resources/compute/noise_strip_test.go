// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// Provider-assigned fields that no forma declares must be dropped from the read
// so they do not read back as perpetual drift. Declared fields must survive.
func TestResponseTransformersDropProviderNoise(t *testing.T) {
	ctx := base.TransformContext{Project: "p", Zone: "us-central1-a"}

	t.Run("disk", func(t *testing.T) {
		out := diskResponseTransformer(map[string]interface{}{
			"name": "d1", "sizeGb": 20, "type": "pd-balanced",
			"sourceImage":               "https://www.googleapis.com/compute/v1/projects/cos-cloud/global/images/cos-1",
			"licenses":                  []interface{}{"x"},
			"guestOsFeatures":           []interface{}{map[string]interface{}{"type": "SEV_CAPABLE"}},
			"architecture":              "X86_64",
			"enableConfidentialCompute": false,
			"physicalBlockSizeBytes":    "4096",
			"lastAttachTimestamp":       "2026-01-01T00:00:00Z",
			"users":                     []interface{}{"projects/p/zones/z/instances/vm"},
		}, ctx)
		for _, k := range []string{"licenses", "guestOsFeatures", "architecture", "enableConfidentialCompute", "physicalBlockSizeBytes", "lastAttachTimestamp", "users"} {
			if _, ok := out[k]; ok {
				t.Errorf("disk: %q not stripped", k)
			}
		}
		if out["name"] != "d1" || out["sourceImage"] != "projects/cos-cloud/global/images/cos-1" {
			t.Errorf("disk: declared fields altered: %#v", out)
		}
	})

	t.Run("network", func(t *testing.T) {
		out := networkResponseTransformer(map[string]interface{}{
			"name": "n1", "autoCreateSubnetworks": false,
			"routingConfig":                         map[string]interface{}{"routingMode": "REGIONAL"},
			"mtu":                                   1460,
			"networkFirewallPolicyEnforcementOrder": "AFTER_CLASSIC_FIREWALL",
			"subnetworks":                           []interface{}{"projects/p/regions/r/subnetworks/s"},
			"peerings":                              []interface{}{map[string]interface{}{"name": "servicenetworking"}},
		}, ctx)
		for _, k := range []string{"routingConfig", "mtu", "networkFirewallPolicyEnforcementOrder", "subnetworks", "peerings"} {
			if _, ok := out[k]; ok {
				t.Errorf("network: %q not stripped", k)
			}
		}
		if out["name"] != "n1" {
			t.Errorf("network: declared field altered: %#v", out)
		}
	})

	t.Run("router", func(t *testing.T) {
		out := routerResponseTransformer(map[string]interface{}{
			"name":                        "r1",
			"region":                      "https://www.googleapis.com/compute/v1/projects/p/regions/us-central1",
			"encryptedInterconnectRouter": false,
			"nats":                        []interface{}{map[string]interface{}{"name": "nat1"}},
		}, ctx)
		for _, k := range []string{"encryptedInterconnectRouter", "nats"} {
			if _, ok := out[k]; ok {
				t.Errorf("router: %q not stripped", k)
			}
		}
		if out["region"] != "us-central1" {
			t.Errorf("router: region not normalized: %#v", out["region"])
		}
	})
}
