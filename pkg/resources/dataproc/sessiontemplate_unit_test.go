// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dataproc

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// Session templates are location-scoped where autoscaling policies are
// region-scoped; mixing the two path builders would 404.
func TestDataprocLocationPathBuilder(t *testing.T) {
	got := dataprocLocationPathBuilder(base.PathContext{
		Project: "dev-1", Location: "europe-central2",
		ResourceType: "sessionTemplates", ResourceName: "st-a",
	})
	if want := "/projects/dev-1/locations/europe-central2/sessionTemplates/st-a"; got != want {
		t.Errorf("resource path: %q", got)
	}
	// A target that configures only a region still resolves.
	got = dataprocLocationPathBuilder(base.PathContext{
		Project: "dev-1", Region: "europe-central2", ResourceType: "sessionTemplates",
	})
	if want := "/projects/dev-1/locations/europe-central2/sessionTemplates"; got != want {
		t.Errorf("region fallback: %q", got)
	}
	// The region-scoped builder must stay as it was.
	got = dataprocPathBuilder(base.PathContext{
		Project: "dev-1", Region: "europe-central2",
		ResourceType: "autoscalingPolicies", ResourceName: "ap-a",
	})
	if want := "/projects/dev-1/regions/europe-central2/autoscalingPolicies/ap-a"; got != want {
		t.Errorf("region path regressed: %q", got)
	}
}

func TestParseDataprocLocationNativeID(t *testing.T) {
	ctx, err := parseDataprocLocationNativeID("projects/dev-1/locations/europe-central2/sessionTemplates/st-a")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Project != "dev-1" || ctx.Location != "europe-central2" ||
		ctx.ResourceType != "sessionTemplates" || ctx.ResourceName != "st-a" {
		t.Errorf("parse: %+v", ctx)
	}
	// Round-trip through the builder is what matters.
	if got := dataprocLocationPathBuilder(ctx); got != "/projects/dev-1/locations/europe-central2/sessionTemplates/st-a" {
		t.Errorf("rebuilt: %q", got)
	}
	for _, bad := range []string{
		"projects/dev-1/regions/europe-central2/sessionTemplates/st-a", // region shape
		"projects/dev-1/locations/europe-central2/sessionTemplates",
		"",
	} {
		if _, err := parseDataprocLocationNativeID(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

// The API binds the id from the body's name and rejects a query parameter, so
// the short name has to be expanded on write and shortened on read. Asymmetry
// would make every comparison report drift.
func TestSessionTemplateNameRoundTrip(t *testing.T) {
	ctx := base.TransformContext{Project: "dev-1", Location: "europe-central2"}
	body, err := sessionTemplateRequestTransformer(map[string]interface{}{
		"name":        "st-a",
		"description": "d",
	}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	full := "projects/dev-1/locations/europe-central2/sessionTemplates/st-a"
	if body["name"] != full {
		t.Errorf("expansion: %v", body["name"])
	}
	out := sessionTemplateResponseTransformer(map[string]interface{}{
		"name": full, "creator": "someone@example.com",
	}, ctx)
	if out["name"] != "st-a" {
		t.Errorf("read should report the short name: %v", out["name"])
	}
	if out["creator"] != "someone@example.com" {
		t.Errorf("other fields must survive: %#v", out)
	}
	// An already-full name is not expanded twice.
	body, _ = sessionTemplateRequestTransformer(map[string]interface{}{"name": full}, ctx)
	if body["name"] != full {
		t.Errorf("double expansion: %v", body["name"])
	}
	// A target with only a region still builds the path.
	body, _ = sessionTemplateRequestTransformer(map[string]interface{}{"name": "st-a"},
		base.TransformContext{Project: "dev-1", Region: "europe-central2"})
	if body["name"] != full {
		t.Errorf("region fallback: %v", body["name"])
	}
}
