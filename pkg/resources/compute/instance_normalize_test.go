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

// A boot disk that does not declare a deviceName gets GCP's auto "persistent-disk-N";
// echoing it back would drift, so it must be dropped.
func TestNormalizeInstanceDisksDropsAutoDeviceName(t *testing.T) {
	out := normalizeInstanceDisks([]interface{}{
		map[string]interface{}{
			"boot":       true,
			"autoDelete": true,
			"source":     "https://www.googleapis.com/compute/v1/projects/p/zones/z/disks/boot",
			"deviceName": "persistent-disk-0",
		},
	})
	if _, ok := out[0].(map[string]interface{})["deviceName"]; ok {
		t.Errorf("auto deviceName should be dropped: %#v", out[0])
	}
}

func TestAutoDeviceName(t *testing.T) {
	for name, want := range map[string]bool{
		"persistent-disk-0":  true,
		"persistent-disk-12": true,
		"tsnet":              false,
		"persistent-disk-":   false,
		"persistent-disk-a":  false,
		"my-persistent-disk-0": false,
	} {
		if got := autoDeviceName(name); got != want {
			t.Errorf("autoDeviceName(%q) = %v, want %v", name, got, want)
		}
	}
}

// GCE returns metadata as items:[{key,value}] + fingerprint/kind; it must be
// normalized back to the schema's items:{key:value} map so it does not drift.
func TestNormalizeInstanceMetadata(t *testing.T) {
	out := normalizeInstanceMetadata(map[string]interface{}{
		"fingerprint": "abc123",
		"kind":        "compute#metadata",
		"items": []interface{}{
			map[string]interface{}{"key": "startup-script", "value": "#!/bin/bash"},
			map[string]interface{}{"key": "foo", "value": "bar"},
		},
	})
	if _, ok := out["fingerprint"]; ok {
		t.Errorf("fingerprint should be dropped: %#v", out)
	}
	items, ok := out["items"].(map[string]interface{})
	if !ok {
		t.Fatalf("items not a map: %#v", out["items"])
	}
	if items["startup-script"] != "#!/bin/bash" || items["foo"] != "bar" {
		t.Errorf("items = %#v", items)
	}
}
