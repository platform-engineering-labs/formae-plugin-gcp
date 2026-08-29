// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package apigateway

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// Apis and their configs are global; only gateways are regional.
func TestPathBuilderLocations(t *testing.T) {
	cases := map[string]base.PathContext{
		"/projects/p/locations/global/apis": {Project: "p", ResourceType: "apis", Location: "eu"},
		"/projects/p/locations/eu/gateways": {Project: "p", ResourceType: "gateways", Location: "eu"},
		"/projects/p/locations/global/apis/a/configs": {
			Project: "p", ResourceType: "configs", ParentResource: "a", Location: "eu",
		},
	}
	for want, ctx := range cases {
		if got := apiGatewayPathBuilder(ctx); got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
	}
}

// A config's OpenAPI documents come back only under the full view, and every
// property a forma declares has to round-trip.
func TestConfigReadAsksForTheFullView(t *testing.T) {
	got := apiGatewayPathBuilder(base.PathContext{
		Project: "p", ResourceType: "configs", ParentResource: "a", ResourceName: "c",
	})
	want := "/projects/p/locations/global/apis/a/configs/c?view=FULL"
	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}

func TestNativeIDRoundTrip(t *testing.T) {
	for _, nativeID := range []string{
		"projects/p/locations/global/apis/a",
		"projects/p/locations/eu/gateways/g",
	} {
		ctx, err := parseAPIGatewayNativeID(nativeID)
		if err != nil {
			t.Fatalf("parse(%q) failed: %v", nativeID, err)
		}
		if got := apiGatewayPathBuilder(ctx); got != "/"+nativeID {
			t.Errorf("round trip = %q, want %q", got, "/"+nativeID)
		}
	}
}

// A fresh operation does not carry the resource; its metadata names the target.
func TestNativeIDFromOperationMetadata(t *testing.T) {
	got := extractAPIGatewayNativeID(map[string]interface{}{
		"name":     "projects/p/locations/global/operations/operation-123",
		"metadata": map[string]interface{}{"target": "projects/p/locations/global/apis/a"},
	}, base.PathContext{})
	if got != "projects/p/locations/global/apis/a" {
		t.Errorf("native ID = %q, want the operation target", got)
	}
}

// The API reports the project number in the path; a forma names the project by
// id, and a config also has to report the full path a gateway refers to.
func TestResponseTransformerPrefersConfiguredProject(t *testing.T) {
	full := "projects/989754770009/locations/global/apis/a/configs/c"
	out := responseTransformer(map[string]interface{}{"name": full}, base.TransformContext{Project: "my-project"})
	if out["project"] != "my-project" {
		t.Errorf("project = %v, want my-project", out["project"])
	}
	if out["api"] != "a" || out["name"] != "c" {
		t.Errorf("api/name = %v/%v, want a/c", out["api"], out["name"])
	}
	if out["path"] != full {
		t.Errorf("path = %v, want %q", out["path"], full)
	}
}
