// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// The API echoes a masked sharedSecret and a sharedSecretHash. Storing either
// would put non-authored secret material in state and break verification of the
// opaque input, so both must be dropped on read.
func TestVpnTunnelResponseDropsSecretMaterial(t *testing.T) {
	out := vpnTunnelResponseTransformer(map[string]interface{}{
		"name":             "t",
		"sharedSecret":     "*************",
		"sharedSecretHash": "AVW2mQxDeadBeef",
		"ikeVersion":       float64(2),
	}, base.TransformContext{})

	for _, k := range []string{"sharedSecret", "sharedSecretHash"} {
		if _, ok := out[k]; ok {
			t.Errorf("%q must never reach stored state: %#v", k, out)
		}
	}
	if out["ikeVersion"] != float64(2) || out["name"] != "t" {
		t.Errorf("other fields must survive: %#v", out)
	}
}

// Regional resources report region as a full URL; the schema declares the short
// name.
func TestVpnTunnelResponseShortensRegion(t *testing.T) {
	out := vpnTunnelResponseTransformer(map[string]interface{}{
		"region": "https://www.googleapis.com/compute/v1/projects/p/regions/europe-central2",
	}, base.TransformContext{})
	if out["region"] != "europe-central2" {
		t.Errorf("region not shortened: %#v", out["region"])
	}
}
