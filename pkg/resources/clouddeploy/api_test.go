//go:build unit

// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package clouddeploy

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

func TestPathBuilderTopLevelCollection(t *testing.T) {
	got := cloudDeployPathBuilder(base.PathContext{
		Project:      "p1",
		Location:     "europe-central2",
		ResourceType: "targets",
		ResourceName: "tgt1",
	})
	want := "/projects/p1/locations/europe-central2/targets/tgt1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A target that configures only a region still has to address a location: Cloud
// Deploy has no locations/global to fall back to.
func TestPathBuilderFallsBackToRegion(t *testing.T) {
	got := cloudDeployPathBuilder(base.PathContext{
		Project:      "p1",
		Region:       "europe-west1",
		ResourceType: "deliveryPipelines",
	})
	want := "/projects/p1/locations/europe-west1/deliveryPipelines"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPathBuilderNestedAutomation(t *testing.T) {
	got := cloudDeployPathBuilder(base.PathContext{
		Project:        "p1",
		Location:       "europe-central2",
		ParentType:     "deliveryPipelines",
		ParentResource: "dp1",
		ResourceType:   "automations",
		ResourceName:   "auto1",
	})
	want := "/projects/p1/locations/europe-central2/deliveryPipelines/dp1/automations/auto1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Discovery lists automations with no parent to walk from. Cloud Deploy accepts
// "-" as a wildcard pipeline, so the list enumerates every automation in the
// region; without this the URL addresses a collection that does not exist and
// the resource never appears in inventory.
func TestPathBuilderWildcardsAParentlessList(t *testing.T) {
	got := cloudDeployPathBuilder(base.PathContext{
		Project:      "p1",
		Location:     "europe-central2",
		ResourceType: "automations",
	})
	want := "/projects/p1/locations/europe-central2/deliveryPipelines/-/automations"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseNativeIDTopLevel(t *testing.T) {
	ctx, err := parseCloudDeployNativeID("projects/p1/locations/europe-central2/targets/tgt1")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ctx.Project != "p1" || ctx.Location != "europe-central2" ||
		ctx.ResourceType != "targets" || ctx.ResourceName != "tgt1" {
		t.Errorf("unexpected context: %+v", ctx)
	}
	if ctx.ParentType != "" {
		t.Errorf("unexpected parent: %+v", ctx)
	}
}

// Without the nested case the pipeline is dropped and the read addresses
// ".../locations/{region}/automations/{automation}", which 404s.
func TestParseNativeIDNestedAutomation(t *testing.T) {
	ctx, err := parseCloudDeployNativeID(
		"projects/p1/locations/europe-central2/deliveryPipelines/dp1/automations/auto1")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ctx.ParentType != "deliveryPipelines" || ctx.ParentResource != "dp1" {
		t.Errorf("parent not recovered: %+v", ctx)
	}
	if ctx.ResourceType != "automations" || ctx.ResourceName != "auto1" {
		t.Errorf("resource not recovered: %+v", ctx)
	}
}

func TestParseNativeIDRejectsGarbage(t *testing.T) {
	for _, id := range []string{"", "targets/tgt1", "projects/p1/locations/l/targets/tgt1/extra"} {
		if _, err := parseCloudDeployNativeID(id); err == nil {
			t.Errorf("expected an error for %q", id)
		}
	}
}

// A Cloud Deploy operation can report done and still carry an error - observed
// live, on a create whose pipeline was never created. Treating done as success
// would report a create that failed as one that worked.
func TestOperationStatusFailsOnDoneWithError(t *testing.T) {
	done, err := checkOperationStatus(map[string]interface{}{
		"done":  true,
		"error": map[string]interface{}{"message": "not a valid resource ID"},
	})
	if !done {
		t.Error("expected done")
	}
	if err == nil {
		t.Fatal("expected an error")
	}
	if err.Error() != "not a valid resource ID" {
		t.Errorf("unexpected message: %v", err)
	}
}

func TestOperationStatusPending(t *testing.T) {
	done, err := checkOperationStatus(map[string]interface{}{"done": false})
	if done || err != nil {
		t.Errorf("expected pending, got done=%v err=%v", done, err)
	}
}

// On async create the response is an Operation, so the native ID has to be
// built from context - including the parent for a nested automation.
func TestNativeIDExtractorFromContext(t *testing.T) {
	got := extractCloudDeployNativeID(map[string]interface{}{
		"name": "projects/p1/locations/europe-central2/operations/op1",
	}, base.PathContext{
		Project:        "p1",
		Location:       "europe-central2",
		ParentType:     "deliveryPipelines",
		ParentResource: "dp1",
		ResourceType:   "automations",
		ResourceName:   "auto1",
	})
	want := "projects/p1/locations/europe-central2/deliveryPipelines/dp1/automations/auto1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A list item carries its own full path, parent segments included.
func TestNativeIDExtractorFromResponse(t *testing.T) {
	got := extractCloudDeployNativeID(map[string]interface{}{
		"name": "projects/p1/locations/europe-central2/deliveryPipelines/dp1/automations/auto1",
	}, base.PathContext{})
	want := "projects/p1/locations/europe-central2/deliveryPipelines/dp1/automations/auto1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Falling back to the operation's metadata.target, which Cloud Deploy fills
// with the resource being acted on.
func TestNativeIDExtractorFromOperationMetadata(t *testing.T) {
	got := extractCloudDeployNativeID(map[string]interface{}{
		"name": "projects/p1/locations/europe-central2/operations/op1",
		"metadata": map[string]interface{}{
			"target": "projects/p1/locations/europe-central2/targets/tgt1",
		},
	}, base.PathContext{})
	want := "projects/p1/locations/europe-central2/targets/tgt1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
