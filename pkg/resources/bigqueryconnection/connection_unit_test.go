// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigqueryconnection

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// The API reports the project *number* in the path it returns, while a forma
// names the project by id. The service account BigQuery mints arrives nested,
// where no hint can mark it as server-filled, so it is lifted to a top-level
// field that can be.
func TestResponseLiftsServiceAccountAndKeepsConfiguredProject(t *testing.T) {
	out := responseTransformer(map[string]interface{}{
		"name":          "projects/989754770009/locations/eu/connections/c1",
		"friendlyName":  "conn",
		"cloudResource": map[string]interface{}{"serviceAccountId": "bqcx-1@gcp-sa-bigquery-condel.iam.gserviceaccount.com"},
	}, base.TransformContext{Project: "my-project"})

	if out["project"] != "my-project" {
		t.Errorf("project = %v, want the configured id", out["project"])
	}
	if out["location"] != "eu" || out["name"] != "c1" {
		t.Errorf("location/name = %v/%v", out["location"], out["name"])
	}
	if out["cloudResourceServiceAccountId"] != "bqcx-1@gcp-sa-bigquery-condel.iam.gserviceaccount.com" {
		t.Errorf("service account was not lifted: %v", out["cloudResourceServiceAccountId"])
	}
	cr, ok := out["cloudResource"].(map[string]interface{})
	if !ok || len(cr) != 0 {
		t.Errorf("cloudResource should be left empty, got %v", out["cloudResource"])
	}
}

// The lifted field is server-reported and must never be sent back.
func TestRequestDropsLiftedServiceAccount(t *testing.T) {
	body, err := requestTransformer(map[string]interface{}{
		"name":                          "c1",
		"project":                       "p",
		"location":                      "eu",
		"cloudResourceServiceAccountId": "bqcx-1@example",
		"friendlyName":                  "conn",
	}, base.TransformContext{})
	if err != nil {
		t.Fatalf("transform failed: %v", err)
	}
	for _, dropped := range []string{"project", "location", "cloudResourceServiceAccountId"} {
		if _, ok := body[dropped]; ok {
			t.Errorf("%s must not be sent", dropped)
		}
	}
	if body["name"] != "c1" {
		t.Error("name must survive a create for the id parameter")
	}
}

// The native ID is where a later read gets its path context, so a project
// number left in it comes back as the project on every sync. Both forms address
// the resource, so the configured id is used.
func TestNativeIDKeepsTheConfiguredProject(t *testing.T) {
	got := extractConnectionNativeID(
		map[string]interface{}{"name": "projects/989754770009/locations/eu/connections/c1"},
		base.PathContext{Project: "my-project"},
	)
	if got != "projects/my-project/locations/eu/connections/c1" {
		t.Errorf("native ID = %q", got)
	}
	// With no configured project there is nothing better than what the API said.
	got = extractConnectionNativeID(
		map[string]interface{}{"name": "projects/989754770009/locations/eu/connections/c1"},
		base.PathContext{},
	)
	if got != "projects/989754770009/locations/eu/connections/c1" {
		t.Errorf("native ID = %q", got)
	}
}

// The update mask is built from the body, and both the id and the connection
// kind are fixed at create: naming either asks the API to change something it
// will not.
func TestUpdateBodyDropsImmutableFields(t *testing.T) {
	body, err := requestTransformer(map[string]interface{}{
		"name":          "c1",
		"cloudResource": map[string]interface{}{},
		"friendlyName":  "renamed",
		"description":   "still here",
	}, base.TransformContext{Operation: resource.OperationUpdate})
	if err != nil {
		t.Fatalf("transform failed: %v", err)
	}
	for _, dropped := range []string{"name", "cloudResource"} {
		if _, ok := body[dropped]; ok {
			t.Errorf("%s must not be in an update body", dropped)
		}
	}
	if body["friendlyName"] != "renamed" || body["description"] != "still here" {
		t.Errorf("mutable fields were dropped: %v", body)
	}
}

// A create still needs both.
func TestCreateBodyKeepsNameAndKind(t *testing.T) {
	body, err := requestTransformer(map[string]interface{}{
		"name":          "c1",
		"cloudResource": map[string]interface{}{},
	}, base.TransformContext{Operation: resource.OperationCreate})
	if err != nil {
		t.Fatalf("transform failed: %v", err)
	}
	if body["name"] != "c1" {
		t.Error("name must survive a create for the id parameter")
	}
	if _, ok := body["cloudResource"]; !ok {
		t.Error("cloudResource selects the connection kind and must survive a create")
	}
}
