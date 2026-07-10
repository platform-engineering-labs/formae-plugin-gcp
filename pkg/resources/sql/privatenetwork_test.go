// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package sql

import "testing"

// Cloud SQL echoes privateNetwork as a full compute URL; it must be normalized
// to the "projects/..." path so it round-trips against a network resolvable
// reference instead of drifting.
func TestPrivateNetworkNormalized(t *testing.T) {
	full := "https://www.googleapis.com/compute/v1/projects/p/global/networks/vpc"
	short := "projects/p/global/networks/vpc"

	out := databaseInstanceResponseTransformer(map[string]interface{}{
		"settings": map[string]interface{}{
			"ipConfiguration": map[string]interface{}{"privateNetwork": full},
		},
	})
	got := out["settings"].(map[string]interface{})["ipConfiguration"].(map[string]interface{})["privateNetwork"]
	if got != short {
		t.Errorf("privateNetwork = %q, want %q", got, short)
	}

	// Already-short value is left unchanged.
	out2 := databaseInstanceResponseTransformer(map[string]interface{}{
		"settings": map[string]interface{}{
			"ipConfiguration": map[string]interface{}{"privateNetwork": short},
		},
	})
	if g := out2["settings"].(map[string]interface{})["ipConfiguration"].(map[string]interface{})["privateNetwork"]; g != short {
		t.Errorf("short privateNetwork altered: %q", g)
	}
}
