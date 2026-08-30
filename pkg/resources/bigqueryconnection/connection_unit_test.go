// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigqueryconnection

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
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
