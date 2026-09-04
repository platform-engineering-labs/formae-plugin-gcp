//go:build unit

// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package clouddeploy

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func createCtx() base.TransformContext {
	return base.TransformContext{
		Project:      "p1",
		Location:     "europe-central2",
		ResourceType: "targets",
		Operation:    resource.OperationCreate,
	}
}

// A forma names a CustomTargetType by its short id, because that is all a
// resolvable can yield. The request has to carry the full path the API
// documents.
func TestTargetRequestExpandsCustomTargetType(t *testing.T) {
	out, err := targetRequestTransformer(map[string]interface{}{
		"name":         "tgt1",
		"customTarget": map[string]interface{}{"customTargetType": "ctt1"},
	}, createCtx())
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	ct := out["customTarget"].(map[string]interface{})
	want := "projects/p1/locations/europe-central2/customTargetTypes/ctt1"
	if got := ct["customTargetType"]; got != want {
		t.Errorf("customTargetType = %v, want %v", got, want)
	}
}

// A value that is already a path is left alone, so a forma may still name a
// type in another project explicitly.
func TestTargetRequestLeavesFullPathAlone(t *testing.T) {
	full := "projects/other/locations/europe-west1/customTargetTypes/ctt1"
	out, err := targetRequestTransformer(map[string]interface{}{
		"name":         "tgt1",
		"customTarget": map[string]interface{}{"customTargetType": full},
	}, createCtx())
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	ct := out["customTarget"].(map[string]interface{})
	if got := ct["customTargetType"]; got != full {
		t.Errorf("customTargetType = %v, want %v", got, full)
	}
}

// The response half shortens both the reference and the target's own name.
func TestTargetResponseShortens(t *testing.T) {
	out := targetResponseTransformer.Transform(map[string]interface{}{
		"name": "projects/p1/locations/europe-central2/targets/tgt1",
		"customTarget": map[string]interface{}{
			"customTargetType": "projects/989754770009/locations/europe-central2/customTargetTypes/ctt1",
		},
	}, base.TransformContext{Project: "p1", Location: "europe-central2"})

	if out["name"] != "tgt1" {
		t.Errorf("name = %v, want tgt1", out["name"])
	}
	ct := out["customTarget"].(map[string]interface{})
	if ct["customTargetType"] != "ctt1" {
		t.Errorf("customTargetType = %v, want ctt1", ct["customTargetType"])
	}
}

// The two halves have to be exact inverses. Expanding without shortening leaves
// declared state and stored state permanently disagreeing on an immutable
// field, so every re-apply plans a replacement that then fails.
func TestTargetCustomTargetTypeRoundTrips(t *testing.T) {
	declared := map[string]interface{}{
		"name":         "tgt1",
		"customTarget": map[string]interface{}{"customTargetType": "ctt1"},
	}
	sent, err := targetRequestTransformer(map[string]interface{}{
		"name":         "tgt1",
		"customTarget": map[string]interface{}{"customTargetType": "ctt1"},
	}, createCtx())
	if err != nil {
		t.Fatalf("transform: %v", err)
	}

	// What the API echoes back is what it was sent, with its own name expanded
	// to a full path - observed live.
	echoed := map[string]interface{}{
		"name":         "projects/p1/locations/europe-central2/targets/tgt1",
		"customTarget": sent["customTarget"],
	}
	got := targetResponseTransformer.Transform(echoed,
		base.TransformContext{Project: "p1", Location: "europe-central2"})

	if got["name"] != declared["name"] {
		t.Errorf("name did not round trip: %v", got["name"])
	}
	gotRef := got["customTarget"].(map[string]interface{})["customTargetType"]
	wantRef := declared["customTarget"].(map[string]interface{})["customTargetType"]
	if gotRef != wantRef {
		t.Errorf("customTargetType did not round trip: got %v, want %v", gotRef, wantRef)
	}
}

// name is the path and the update mask is built from the body, so it must not
// survive into a patch.
func TestTargetRequestDropsNameOnUpdate(t *testing.T) {
	ctx := createCtx()
	ctx.Operation = resource.OperationUpdate
	out, err := targetRequestTransformer(map[string]interface{}{
		"name":        "tgt1",
		"description": "d",
	}, ctx)
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	if _, ok := out["name"]; ok {
		t.Error("name should be dropped from an update body")
	}
	if out["description"] != "d" {
		t.Error("description should survive an update body")
	}
}
