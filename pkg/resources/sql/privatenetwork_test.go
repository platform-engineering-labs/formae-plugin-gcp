// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package sql

import "testing"

// The forma declares privateNetwork from a network resolvable's .selfLink (full
// compute URL), but Cloud SQL stores the bare "projects/..." path. The read must
// expand it back to the selfLink form so it round-trips instead of drifting.
func TestPrivateNetworkNormalized(t *testing.T) {
	full := "https://www.googleapis.com/compute/v1/projects/p/global/networks/vpc"
	short := "projects/p/global/networks/vpc"

	out := databaseInstanceResponseTransformer(map[string]interface{}{
		"settings": map[string]interface{}{
			"ipConfiguration": map[string]interface{}{"privateNetwork": short},
		},
	})
	got := out["settings"].(map[string]interface{})["ipConfiguration"].(map[string]interface{})["privateNetwork"]
	if got != full {
		t.Errorf("privateNetwork = %q, want %q", got, full)
	}

	// Already-full value is left unchanged.
	out2 := databaseInstanceResponseTransformer(map[string]interface{}{
		"settings": map[string]interface{}{
			"ipConfiguration": map[string]interface{}{"privateNetwork": full},
		},
	})
	if g := out2["settings"].(map[string]interface{})["ipConfiguration"].(map[string]interface{})["privateNetwork"]; g != full {
		t.Errorf("full privateNetwork altered: %q", g)
	}
}
