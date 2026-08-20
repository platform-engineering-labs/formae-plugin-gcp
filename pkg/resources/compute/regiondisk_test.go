// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

var regionDiskCtx = base.TransformContext{Project: "p", Region: "europe-central2"}

// The API rejects a bare "pd-balanced" ("The URL is malformed") and short
// replica zones, so both must be expanded before the request goes out.
func TestRegionDiskRequestExpandsShortForms(t *testing.T) {
	out, err := regionDiskRequestTransformer(map[string]interface{}{
		"name":         "d",
		"type":         "pd-balanced",
		"replicaZones": []interface{}{"europe-central2-a", "europe-central2-b"},
	}, regionDiskCtx)
	if err != nil {
		t.Fatal(err)
	}
	if out["type"] != "projects/p/regions/europe-central2/diskTypes/pd-balanced" {
		t.Errorf("type not expanded: %#v", out["type"])
	}
	want := []interface{}{
		"https://www.googleapis.com/compute/v1/projects/p/zones/europe-central2-a",
		"https://www.googleapis.com/compute/v1/projects/p/zones/europe-central2-b",
	}
	if !reflect.DeepEqual(out["replicaZones"], want) {
		t.Errorf("replicaZones not expanded: %#v", out["replicaZones"])
	}
}

// A caller who already wrote full URLs must not have them mangled.
func TestRegionDiskRequestLeavesFullFormsAlone(t *testing.T) {
	fullType := "projects/p/regions/europe-central2/diskTypes/pd-ssd"
	fullZone := "https://www.googleapis.com/compute/v1/projects/p/zones/europe-central2-a"
	out, err := regionDiskRequestTransformer(map[string]interface{}{
		"type":         fullType,
		"replicaZones": []interface{}{fullZone},
	}, regionDiskCtx)
	if err != nil {
		t.Fatal(err)
	}
	if out["type"] != fullType {
		t.Errorf("type rewritten: %#v", out["type"])
	}
	if !reflect.DeepEqual(out["replicaZones"], []interface{}{fullZone}) {
		t.Errorf("replicaZones rewritten: %#v", out["replicaZones"])
	}
}

// Read must shorten every URL the API echoes, or Verify reports drift against
// the short forms the forma declared.
func TestRegionDiskResponseShortensURLs(t *testing.T) {
	out := regionDiskResponseTransformer(map[string]interface{}{
		"name":   "d",
		"region": "https://www.googleapis.com/compute/v1/projects/p/regions/europe-central2",
		"type":   "https://www.googleapis.com/compute/v1/projects/p/regions/europe-central2/diskTypes/pd-balanced",
		"replicaZones": []interface{}{
			"https://www.googleapis.com/compute/v1/projects/p/zones/europe-central2-a",
			"https://www.googleapis.com/compute/v1/projects/p/zones/europe-central2-b",
		},
	}, regionDiskCtx)
	if out["region"] != "europe-central2" {
		t.Errorf("region not shortened: %#v", out["region"])
	}
	if out["type"] != "pd-balanced" {
		t.Errorf("type not shortened: %#v", out["type"])
	}
	want := []interface{}{"europe-central2-a", "europe-central2-b"}
	if !reflect.DeepEqual(out["replicaZones"], want) {
		t.Errorf("replicaZones not shortened: %#v", out["replicaZones"])
	}
	// The response carries no project; the schema declares one.
	if out["project"] != "p" {
		t.Errorf("project not restored: %#v", out["project"])
	}
}
