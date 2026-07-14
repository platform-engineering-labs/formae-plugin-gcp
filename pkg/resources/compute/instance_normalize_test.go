// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

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

// The schema mirrors the API metadata shape, so the response transformer keeps
// items (array) and fingerprint as-is and only drops the "kind" discriminator.
func TestInstanceResponseTransformerMetadata(t *testing.T) {
	out := instanceResponseTransformer(map[string]interface{}{
		"metadata": map[string]interface{}{
			"fingerprint": "abc123",
			"kind":        "compute#metadata",
			"items": []interface{}{
				map[string]interface{}{"key": "startup-script", "value": "#!/bin/bash"},
			},
		},
	}, base.TransformContext{})
	md := out["metadata"].(map[string]interface{})
	if _, ok := md["kind"]; ok {
		t.Errorf("kind should be dropped: %#v", md)
	}
	if md["fingerprint"] != "abc123" {
		t.Errorf("fingerprint should round-trip: %#v", md)
	}
	items, ok := md["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("items should stay an array: %#v", md["items"])
	}
	if items[0].(map[string]interface{})["key"] != "startup-script" {
		t.Errorf("items entry wrong: %#v", items[0])
	}
}
