// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package artifactregistry

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// A rule lives under its repository, so the path builder has to insert the
// parent — without it the request would hit the location-level collection.
func TestRulePathBuilderInsertsParent(t *testing.T) {
	got := artifactRegistryPathBuilder(base.PathContext{
		Project: "dev-1", Location: "europe-central2",
		ParentType: "repositories", ParentResource: "repo-a",
		ResourceType: "rules", ResourceName: "rule-a",
	})
	want := "/projects/dev-1/locations/europe-central2/repositories/repo-a/rules/rule-a"
	if got != want {
		t.Errorf("nested resource path: %q", got)
	}

	// Collection URL for a create/list.
	got = artifactRegistryPathBuilder(base.PathContext{
		Project: "dev-1", Location: "europe-central2",
		ParentType: "repositories", ParentResource: "repo-a",
		ResourceType: "rules",
	})
	if want = "/projects/dev-1/locations/europe-central2/repositories/repo-a/rules"; got != want {
		t.Errorf("collection path: %q", got)
	}

	// A repository has no parent and must keep its old shape.
	got = artifactRegistryPathBuilder(base.PathContext{
		Project: "dev-1", Location: "europe-central2",
		ResourceType: "repositories", ResourceName: "repo-a",
	})
	if want = "/projects/dev-1/locations/europe-central2/repositories/repo-a"; got != want {
		t.Errorf("unparented path regressed: %q", got)
	}
}

// A rule reports only its full name plus action/operation, so repository and
// location have to be recovered from the path or a forma declaring them would
// see them as missing.
func TestRuleResponseTransformerRecoversPathFields(t *testing.T) {
	out := ruleResponseTransformer(map[string]interface{}{
		"name":      "projects/dev-1/locations/europe-central2/repositories/repo-a/rules/rule-a",
		"action":    "DENY",
		"operation": "DOWNLOAD",
	}, base.TransformContext{})

	if out["name"] != "rule-a" {
		t.Errorf("name should shorten: %v", out["name"])
	}
	if out["repository"] != "repo-a" || out["location"] != "europe-central2" {
		t.Errorf("path fields: %#v", out)
	}
	if out["action"] != "DENY" || out["operation"] != "DOWNLOAD" {
		t.Errorf("rule fields must survive: %#v", out)
	}

	// An unexpected name shape must pass through untouched rather than produce
	// half-parsed values.
	odd := ruleResponseTransformer(map[string]interface{}{"name": "rule-a"}, base.TransformContext{})
	if odd["name"] != "rule-a" {
		t.Errorf("short name mangled: %#v", odd)
	}
	if _, ok := odd["repository"]; ok {
		t.Errorf("repository invented from a short name: %#v", odd)
	}
}

// repository and location address the URL; the API rejects either as a body
// field, and name travels in ?ruleId= rather than the payload.
func TestRuleRequestTransformerDropsPathProps(t *testing.T) {
	body, err := ruleRequestTransformer(map[string]interface{}{
		"name":       "rule-a",
		"repository": "repo-a",
		"location":   "europe-central2",
		"action":     "DENY",
		"operation":  "DOWNLOAD",
		"condition":  map[string]interface{}{"expression": "x"},
	}, base.TransformContext{})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"repository", "location"} {
		if _, ok := body[k]; ok {
			t.Errorf("%s must not reach the body: %#v", k, body)
		}
	}
	// name must survive: the engine reads ?ruleId= from the transformed body,
	// so dropping it sends an empty id and the API rejects the rule name.
	if body["name"] != "rule-a" {
		t.Errorf("name must stay in the body for the create id: %#v", body)
	}
	if body["action"] != "DENY" || body["operation"] != "DOWNLOAD" {
		t.Errorf("rule fields must survive: %#v", body)
	}
	if _, ok := body["condition"]; !ok {
		t.Errorf("condition must survive: %#v", body)
	}
}

// A nested resource's native ID must keep its parent segment: the read URL is
// built from the id, so dropping the repository would address the
// location-level collection and 404.
func TestNativeIDKeepsParentSegment(t *testing.T) {
	got := extractArtifactRegistryNativeID(map[string]interface{}{}, base.PathContext{
		Project: "dev-1", Location: "europe-central2",
		ParentType: "repositories", ParentResource: "repo-a",
		ResourceType: "rules", ResourceName: "rule-a",
	})
	want := "projects/dev-1/locations/europe-central2/repositories/repo-a/rules/rule-a"
	if got != want {
		t.Errorf("nested native ID: %q", got)
	}

	// An unparented resource keeps its old shape.
	got = extractArtifactRegistryNativeID(map[string]interface{}{}, base.PathContext{
		Project: "dev-1", Location: "europe-central2",
		ResourceType: "repositories", ResourceName: "repo-a",
	})
	if want = "projects/dev-1/locations/europe-central2/repositories/repo-a"; got != want {
		t.Errorf("unparented native ID regressed: %q", got)
	}
}

// The read URL is rebuilt from the native ID, so the parser has to restore the
// parent for a nested resource and leave a plain repository alone.
func TestParseNativeIDRestoresParent(t *testing.T) {
	ctx, err := parseArtifactRegistryNativeID(
		"projects/dev-1/locations/europe-central2/repositories/repo-a/rules/rule-a")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.ParentType != "repositories" || ctx.ParentResource != "repo-a" ||
		ctx.ResourceType != "rules" || ctx.ResourceName != "rule-a" ||
		ctx.Project != "dev-1" || ctx.Location != "europe-central2" {
		t.Errorf("nested parse: %+v", ctx)
	}
	// Round-trip through the path builder is what actually matters.
	if got := artifactRegistryPathBuilder(ctx); got !=
		"/projects/dev-1/locations/europe-central2/repositories/repo-a/rules/rule-a" {
		t.Errorf("rebuilt URL: %q", got)
	}

	ctx, err = parseArtifactRegistryNativeID("projects/dev-1/locations/europe-central2/repositories/repo-a")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.ParentType != "" || ctx.ResourceType != "repositories" || ctx.ResourceName != "repo-a" {
		t.Errorf("unparented parse: %+v", ctx)
	}

	for _, bad := range []string{
		"projects/dev-1/locations/europe-central2/repositories",
		"projects/dev-1/locations/europe-central2/repositories/repo-a/rules",
		"projects/dev-1/repositories/repo-a",
		"",
	} {
		if _, err := parseArtifactRegistryNativeID(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}
