// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package bigquery

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// TestFilterPredicateSurvivesGoJSONRoundTrip pins the one thing about this type
// that looks like an API defect and is not.
//
// BigQuery answers with the predicate HTML-escaped - `v \u003e 0` where the
// forma declared `v > 0` - and Go's encoding/json escapes the same characters on
// the way out, so the bytes on the wire differ from what a forma declared in both
// directions. They are JSON escapes, not a rewritten predicate: the value decodes
// back to exactly what was sent.
//
// This matters because filterPredicate is the type's only mutable field. Had the
// API actually normalised it - collapsing whitespace, adding parens, upcasing
// operators - declared and stored state would disagree permanently and every
// sync would report drift on a field nothing had touched.
//
// Verified against the live API as well as here: a predicate sent with a double
// space (`v BETWEEN 1 AND  5`) came back with the double space intact.
func TestFilterPredicateSurvivesGoJSONRoundTrip(t *testing.T) {
	predicates := []string{
		`v > 0`,
		`v > 0 AND region <> "US" AND v <= 5`,
		`region = "EU" AND v BETWEEN 1 AND  5`,
		`nullable_field is not NULL`,
		`(v > 0) OR (v < -10)`,
		`date_field = CAST('2019-9-27' as DATE)`,
	}

	for _, predicate := range predicates {
		encoded, err := json.Marshal(map[string]interface{}{"filterPredicate": predicate})
		if err != nil {
			t.Fatalf("marshal %q: %v", predicate, err)
		}

		var decoded map[string]interface{}
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("unmarshal %q: %v", encoded, err)
		}

		got, _ := decoded["filterPredicate"].(string)
		if got != predicate {
			t.Errorf("filterPredicate did not round trip byte-for-byte\n  sent: %q\n  got : %q\n  wire: %s",
				predicate, got, encoded)
		}
	}
}

// TestFilterPredicateWireFormIsEscapedNotRewritten records the escaping itself,
// so that a future reader who sees `\u003e` in a request log does not go
// looking for a bug in the transformers.
func TestFilterPredicateWireFormIsEscapedNotRewritten(t *testing.T) {
	encoded, err := json.Marshal(map[string]interface{}{"filterPredicate": `v > 0`})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Built rather than written literally: a source file that spelled the escape
	// out would itself be ambiguous about which of the two forms it holds.
	escaped := `\` + `u003e`
	if !strings.Contains(string(encoded), escaped) {
		t.Fatalf("expected Go to escape '>' as %s on the wire, got %s", escaped, encoded)
	}
}

func TestRowAccessPolicyRequestTransformer(t *testing.T) {
	tests := []struct {
		name      string
		operation resource.Operation
		props     map[string]interface{}
		want      map[string]interface{}
	}{
		{
			name:      "create builds the composite reference and drops the flat identity",
			operation: resource.OperationCreate,
			props: map[string]interface{}{
				"project":         "proj-1",
				"datasetId":       "ds_1",
				"tableId":         "tbl_1",
				"name":            "pol_1",
				"filterPredicate": `v > 0`,
			},
			want: map[string]interface{}{
				"filterPredicate": `v > 0`,
				"rowAccessPolicyReference": map[string]interface{}{
					"projectId": "proj-1",
					"datasetId": "ds_1",
					"tableId":   "tbl_1",
					"policyId":  "pol_1",
				},
			},
		},
		{
			// The update is a PUT and the API requires the reference on it too:
			// without it the call fails with "Project ID in URL path and content
			// do not match."
			name:      "update builds the reference as well",
			operation: resource.OperationUpdate,
			props: map[string]interface{}{
				"project":         "proj-1",
				"datasetId":       "ds_1",
				"tableId":         "tbl_1",
				"name":            "pol_1",
				"filterPredicate": `v > 100`,
			},
			want: map[string]interface{}{
				"filterPredicate": `v > 100`,
				"rowAccessPolicyReference": map[string]interface{}{
					"projectId": "proj-1",
					"datasetId": "ds_1",
					"tableId":   "tbl_1",
					"policyId":  "pol_1",
				},
			},
		},
		{
			// The URL is built from the target's project, so the reference has
			// to name that same project or the API rejects the mismatch.
			name:      "the context's project wins over a declared one",
			operation: resource.OperationCreate,
			props: map[string]interface{}{
				"project":         "declared-elsewhere",
				"datasetId":       "ds_1",
				"tableId":         "tbl_1",
				"name":            "pol_1",
				"filterPredicate": `TRUE`,
			},
			want: map[string]interface{}{
				"filterPredicate": `TRUE`,
				"rowAccessPolicyReference": map[string]interface{}{
					"projectId": "proj-1",
					"datasetId": "ds_1",
					"tableId":   "tbl_1",
					"policyId":  "pol_1",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rowAccessPolicyRequestTransformer(tt.props, base.TransformContext{
				Project:   "proj-1",
				Operation: tt.operation,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("transformed body mismatch\n  got : %#v\n  want: %#v", got, tt.want)
			}
		})
	}
}

func TestRowAccessPolicyRequestTransformerRefusesIncompleteIdentity(t *testing.T) {
	complete := map[string]interface{}{
		"datasetId":       "ds_1",
		"tableId":         "tbl_1",
		"name":            "pol_1",
		"filterPredicate": `v > 0`,
	}

	for _, missing := range []string{"datasetId", "tableId", "name"} {
		props := make(map[string]interface{}, len(complete))
		for k, v := range complete {
			if k == missing {
				continue
			}
			props[k] = v
		}
		if _, err := rowAccessPolicyRequestTransformer(props,
			base.TransformContext{Project: "proj-1"}); err == nil {
			t.Errorf("expected an error when %q is missing, got none", missing)
		}
	}

	// No project anywhere: the URL cannot be addressed either.
	if _, err := rowAccessPolicyRequestTransformer(complete, base.TransformContext{}); err == nil {
		t.Error("expected an error when the project is unknown, got none")
	}
}

func TestRowAccessPolicyResponseTransformer(t *testing.T) {
	// The exact shape of a live GET, escapes decoded.
	apiResponse := map[string]interface{}{
		"rowAccessPolicyReference": map[string]interface{}{
			"projectId": "proj-1",
			"datasetId": "ds_1",
			"tableId":   "tbl_1",
			"policyId":  "pol_1",
		},
		"filterPredicate":  `v > 0`,
		"creationTime":     "2026-09-03T15:07:31.271482Z",
		"lastModifiedTime": "2026-09-03T15:07:31.271482Z",
		"etag":             "BwZalYLLUZw=",
	}

	want := map[string]interface{}{
		"project":         "proj-1",
		"datasetId":       "ds_1",
		"tableId":         "tbl_1",
		"name":            "pol_1",
		"filterPredicate": `v > 0`,
	}

	got := rowAccessPolicyResponseTransformer(apiResponse, base.TransformContext{Project: "proj-1"})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("transformed response mismatch\n  got : %#v\n  want: %#v", got, want)
	}
}

// TestRowAccessPolicyIdentityRoundTrips is the identity pin required of any
// field expanded on the way out and shortened on the way back. Every one of the
// four identity properties is immutable, so if the request half and the response
// half ever disagree, declared and stored state disagree permanently and each
// re-apply plans a replacement that then fails.
func TestRowAccessPolicyIdentityRoundTrips(t *testing.T) {
	declared := map[string]interface{}{
		"project":         "proj-1",
		"datasetId":       "ds_1",
		"tableId":         "tbl_1",
		"name":            "pol_1",
		"filterPredicate": `v > 0 AND region <> "US"`,
	}

	ctx := base.TransformContext{Project: "proj-1", Operation: resource.OperationCreate}
	body, err := rowAccessPolicyRequestTransformer(declared, ctx)
	if err != nil {
		t.Fatalf("request transform: %v", err)
	}

	// BigQuery echoes the request's reference and adds the output-only fields.
	apiResponse := map[string]interface{}{
		rowAccessPolicyRefField: body[rowAccessPolicyRefField],
		"filterPredicate":       body["filterPredicate"],
		"creationTime":          "2026-09-03T15:07:31.271482Z",
		"lastModifiedTime":      "2026-09-03T15:07:31.271482Z",
	}

	got := rowAccessPolicyResponseTransformer(apiResponse, ctx)
	if !reflect.DeepEqual(got, declared) {
		t.Errorf("identity did not round trip\n  declared: %#v\n  read back: %#v", declared, got)
	}
}

func TestBigQueryPathBuilder(t *testing.T) {
	tests := []struct {
		name string
		ctx  base.PathContext
		want string
	}{
		{
			name: "collection URL for insert and list",
			ctx: base.PathContext{
				Project:        "proj-1",
				CustomSegments: []string{"ds_1"},
				ParentType:     "tables",
				ParentResource: "tbl_1",
				ResourceType:   rowAccessPolicyCollection,
			},
			want: "/projects/proj-1/datasets/ds_1/tables/tbl_1/rowAccessPolicies",
		},
		{
			name: "resource URL for get, update and delete",
			ctx: base.PathContext{
				Project:        "proj-1",
				CustomSegments: []string{"ds_1"},
				ParentType:     "tables",
				ParentResource: "tbl_1",
				ResourceType:   rowAccessPolicyCollection,
				ResourceName:   "pol_1",
			},
			want: "/projects/proj-1/datasets/ds_1/tables/tbl_1/rowAccessPolicies/pol_1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bigQueryPathBuilder(tt.ctx); got != tt.want {
				t.Errorf("path mismatch\n  got : %s\n  want: %s", got, tt.want)
			}
		})
	}
}

func TestParseBigQueryNativeID(t *testing.T) {
	nativeID := "projects/proj-1/datasets/ds_1/tables/tbl_1/rowAccessPolicies/pol_1"
	want := base.PathContext{
		Project:        "proj-1",
		CustomSegments: []string{"ds_1"},
		ParentType:     "tables",
		ParentResource: "tbl_1",
		ResourceType:   rowAccessPolicyCollection,
		ResourceName:   "pol_1",
	}

	got, err := parseBigQueryNativeID(nativeID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsed context mismatch\n  got : %#v\n  want: %#v", got, want)
	}

	// The parser is what keeps both collections above the policy in the URL, so
	// anything that is not the full four-collection path has to be refused
	// rather than silently addressing a shorter one.
	for _, bad := range []string{
		"projects/proj-1/datasets/ds_1/rowAccessPolicies/pol_1",
		"projects/proj-1/datasets/ds_1/tables/tbl_1",
		"projects/proj-1/datasets/ds_1/views/v_1/rowAccessPolicies/pol_1",
		"pol_1",
		"",
	} {
		if _, err := parseBigQueryNativeID(bad); err == nil {
			t.Errorf("expected %q to be rejected, it was accepted", bad)
		}
	}
}

func TestParseBigQueryNativeIDRoundTripsWithPathBuilder(t *testing.T) {
	nativeID := "projects/proj-1/datasets/ds_1/tables/tbl_1/rowAccessPolicies/pol_1"
	ctx, err := parseBigQueryNativeID(nativeID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := bigQueryPathBuilder(ctx); got != "/"+nativeID {
		t.Errorf("native ID did not round trip through the path builder\n  got : %s\n  want: /%s",
			got, nativeID)
	}
}

func TestExtractBigQueryNativeID(t *testing.T) {
	want := "projects/proj-1/datasets/ds_1/tables/tbl_1/rowAccessPolicies/pol_1"

	// A list item and a get/insert/update response both carry the reference and
	// nothing else that identifies the policy - there is no "name" field to fall
	// back to, which is why the extractor reads the reference first.
	fromResponse := extractBigQueryNativeID(map[string]interface{}{
		rowAccessPolicyRefField: map[string]interface{}{
			"projectId": "proj-1",
			"datasetId": "ds_1",
			"tableId":   "tbl_1",
			"policyId":  "pol_1",
		},
	}, base.PathContext{})
	if fromResponse != want {
		t.Errorf("from response: got %q, want %q", fromResponse, want)
	}

	fromContext := extractBigQueryNativeID(map[string]interface{}{}, base.PathContext{
		Project:        "proj-1",
		CustomSegments: []string{"ds_1"},
		ParentType:     "tables",
		ParentResource: "tbl_1",
		ResourceType:   rowAccessPolicyCollection,
		ResourceName:   "pol_1",
	})
	if fromContext != want {
		t.Errorf("from context: got %q, want %q", fromContext, want)
	}

	if got := extractBigQueryNativeID(map[string]interface{}{}, base.PathContext{}); got != "" {
		t.Errorf("with nothing to build from: got %q, want an empty string", got)
	}
}

// TestRowAccessPolicyListWalkerIsRegistered guards the init-ordering trap
// registerRowAccessPolicyListWalker documents: List must resolve to the walking
// provisioner, and every other operation to the generic one. If the override
// were ever registered from an init of its own it would depend on filename
// order, and the failure mode is silent - discovery just never finds a policy.
func TestRowAccessPolicyListWalkerIsRegistered(t *testing.T) {
	for _, operation := range []resource.Operation{
		resource.OperationCreate, resource.OperationRead, resource.OperationUpdate,
		resource.OperationDelete, resource.OperationList, resource.OperationCheckStatus,
	} {
		if !registry.HasProvisioner(RowAccessPolicyResourceType, operation) {
			t.Errorf("%s has no provisioner for %s", RowAccessPolicyResourceType, operation)
		}
	}

	listProvisioner := registry.Get(RowAccessPolicyResourceType, resource.OperationList, &config.Config{})
	if _, ok := listProvisioner.(*rowAccessPolicyListProvisioner); !ok {
		t.Errorf("List resolves to %T, want *rowAccessPolicyListProvisioner - "+
			"the generic List cannot address a policy no caller names a table for", listProvisioner)
	}

	readProvisioner := registry.Get(RowAccessPolicyResourceType, resource.OperationRead, &config.Config{})
	if _, ok := readProvisioner.(*rowAccessPolicyListProvisioner); ok {
		t.Error("Read resolves to the list walker; the override must replace List only")
	}
}
