// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package compute

import (
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// After an update GCP echoes each extensionPolicies entry with a stringConfig it
// was never sent. It is nested inside a mapping value, where hasProviderDefault
// cannot reach it, so every read after the first update would otherwise
// disagree with the declaration and plan a patch that changes nothing.
func TestZoneVmExtensionPolicyStripsEchoedStringConfig(t *testing.T) {
	out := zoneVmExtensionPolicyResponseTransformer(map[string]interface{}{
		"kind":              "compute#vmExtensionPolicy",
		"id":                "123",
		"creationTimestamp": "2026-01-01T00:00:00Z",
		"updateTimestamp":   "2026-01-02T00:00:00Z",
		"selfLink":          "https://www.googleapis.com/compute/v1/projects/p/zones/europe-central2-b/vmExtensionPolicies/zvep",
		"selfLinkWithId":    "https://www.googleapis.com/compute/v1/projects/p/zones/europe-central2-b/vmExtensionPolicies/123",
		"name":              "zvep",
		"description":       "probe",
		"priority":          1000,
		"state":             "ACTIVE",
		"extensionPolicies": map[string]interface{}{
			"ops-agent": map[string]interface{}{
				"pinnedVersion": "",
				"stringConfig":  "",
			},
		},
	}, base.TransformContext{Project: "p", Zone: "europe-central2-b"})

	entry, ok := out["extensionPolicies"].(map[string]interface{})["ops-agent"].(map[string]interface{})
	if !ok {
		t.Fatalf("extensionPolicies entry lost: %#v", out["extensionPolicies"])
	}
	if _, present := entry["stringConfig"]; present {
		t.Errorf("echoed empty stringConfig must be stripped: %#v", entry)
	}

	// pinnedVersion = "" is what a forma says to mean "track current release".
	// It is declared, so stripping it would invent the drift this transformer
	// exists to remove.
	if v, present := entry["pinnedVersion"]; !present || v != "" {
		t.Errorf("declared empty pinnedVersion must survive: %#v", entry)
	}

	// state is a schema field with hasProviderDefault, not noise to strip; id and
	// selfLink back res.id / res.selfLink and must survive too.
	for _, k := range []string{"name", "description", "priority", "state", "id", "selfLink"} {
		if _, present := out[k]; !present {
			t.Errorf("top-level %q must not be stripped", k)
		}
	}
}

// A stringConfig with real content is a declared value and must round-trip.
func TestZoneVmExtensionPolicyKeepsRealStringConfig(t *testing.T) {
	in := map[string]interface{}{
		"name": "zvep",
		"extensionPolicies": map[string]interface{}{
			"ops-agent": map[string]interface{}{
				"pinnedVersion": "1.2.3",
				"stringConfig":  "logging: on",
			},
		},
	}
	out := zoneVmExtensionPolicyResponseTransformer(in, base.TransformContext{})

	want := map[string]interface{}{"pinnedVersion": "1.2.3", "stringConfig": "logging: on"}
	got := out["extensionPolicies"].(map[string]interface{})["ops-agent"]
	if !reflect.DeepEqual(got, want) {
		t.Errorf("declared entry altered:\n got %#v\nwant %#v", got, want)
	}
}

// Discovery reads whatever the API holds, so odd shapes must pass through
// rather than panic.
func TestZoneVmExtensionPolicyToleratesOddShapes(t *testing.T) {
	t.Run("no extensionPolicies", func(t *testing.T) {
		out := zoneVmExtensionPolicyResponseTransformer(
			map[string]interface{}{"name": "zvep"}, base.TransformContext{})
		if _, present := out["extensionPolicies"]; present {
			t.Errorf("absent extensionPolicies must not be invented: %#v", out)
		}
	})

	t.Run("non-object entry", func(t *testing.T) {
		out := zoneVmExtensionPolicyResponseTransformer(map[string]interface{}{
			"extensionPolicies": map[string]interface{}{"ops-agent": "nonsense"},
		}, base.TransformContext{})
		if got := out["extensionPolicies"].(map[string]interface{})["ops-agent"]; got != "nonsense" {
			t.Errorf("non-object entry altered: %#v", got)
		}
	})
}

// The transformer must not write through to the caller's map.
func TestZoneVmExtensionPolicyDoesNotMutateInput(t *testing.T) {
	in := map[string]interface{}{
		"extensionPolicies": map[string]interface{}{
			"ops-agent": map[string]interface{}{"stringConfig": ""},
		},
	}
	zoneVmExtensionPolicyResponseTransformer(in, base.TransformContext{})

	entry := in["extensionPolicies"].(map[string]interface{})["ops-agent"].(map[string]interface{})
	if _, present := entry["stringConfig"]; !present {
		t.Errorf("input map was mutated: %#v", entry)
	}
}
