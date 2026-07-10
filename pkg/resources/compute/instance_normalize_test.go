// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import "testing"

// A declared deviceName must round-trip through the instance disk read so it
// does not drift (the schema marks it hasProviderDefault, which suppresses the
// GCP auto-assigned value for disks that do not declare one).
func TestNormalizeInstanceDisksKeepsDeviceName(t *testing.T) {
	out := normalizeInstanceDisks([]interface{}{
		map[string]interface{}{
			"boot":       false,
			"autoDelete": false,
			"source":     "https://www.googleapis.com/compute/v1/projects/p/zones/z/disks/tsnet",
			"deviceName": "tsnet",
			"index":      1, // API enrichment that must be dropped
		},
	})
	d := out[0].(map[string]interface{})
	if d["deviceName"] != "tsnet" {
		t.Errorf("deviceName not kept: %#v", d)
	}
	if _, ok := d["index"]; ok {
		t.Errorf("API enrichment 'index' should be dropped: %#v", d)
	}
}
