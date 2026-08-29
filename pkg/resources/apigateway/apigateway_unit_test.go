// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package apigateway

import (
	"errors"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
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

// A destroy removes a config before its api, but both deletes are long-running
// and the api's can reach the API while the config is still releasing it. The
// refusal is retryable rather than fatal.
func TestNestedResourceDeleteIsRetryable(t *testing.T) {
	retry := APIGatewayOperations.RetryableError
	if retry == nil {
		t.Fatal("no retryable-error predicate configured")
	}
	if !retry(errors.New("Resource '...apis/a' has nested resources. If the API supports cascading delete, set 'force' to true")) {
		t.Error("a nested-resources refusal should be retried")
	}
	if retry(errors.New("Permission denied")) {
		t.Error("an unrelated failure must not be retried")
	}
	if retry(nil) {
		t.Error("nil must not be retried")
	}
}

// The update mask is built from the body, so an immutable field left in it is
// refused. A config's documents and its gateway identity are both create-only.
func TestUpdateBodyDropsImmutableFields(t *testing.T) {
	body, err := requestTransformer(map[string]interface{}{
		"name":                  "c",
		"api":                   "a",
		"project":               "p",
		"openapiDocuments":      []interface{}{"doc"},
		"gatewayServiceAccount": "sa@example.com",
		"state":                 "ACTIVE",
		"displayName":           "new name",
		"labels":                map[string]interface{}{"a": "b"},
	}, base.TransformContext{Operation: resource.OperationUpdate})
	if err != nil {
		t.Fatalf("transform failed: %v", err)
	}
	for _, dropped := range []string{"name", "api", "project", "openapiDocuments", "gatewayServiceAccount", "state"} {
		if _, ok := body[dropped]; ok {
			t.Errorf("%s must not be in an update body", dropped)
		}
	}
	if body["displayName"] != "new name" || body["labels"] == nil {
		t.Errorf("mutable fields were dropped: %v", body)
	}
}

// A create still needs the documents and the id.
func TestCreateBodyKeepsDocumentsAndName(t *testing.T) {
	body, err := requestTransformer(map[string]interface{}{
		"name":             "c",
		"openapiDocuments": []interface{}{"doc"},
	}, base.TransformContext{Operation: resource.OperationCreate})
	if err != nil {
		t.Fatalf("transform failed: %v", err)
	}
	if body["name"] != "c" || body["openapiDocuments"] == nil {
		t.Errorf("create body is missing what it needs: %v", body)
	}
}
