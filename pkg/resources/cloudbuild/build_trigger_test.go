// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package cloudbuild

import (
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// The service-account reference is expanded on the way out and shortened on the
// way back, so the pair has to be an identity. If it is not, the declared value
// and the stored state disagree forever and every re-apply plans a change that
// is not one.
func TestServiceAccountRefRoundTrip(t *testing.T) {
	for _, email := range []string{
		"formae-tester@development-477117.iam.gserviceaccount.com",
		"989754770009-compute@developer.gserviceaccount.com",
		"sa@other-project.iam.gserviceaccount.com",
	} {
		expanded := expandServiceAccountRef(email, "development-477117")
		if expanded == email {
			t.Fatalf("expandServiceAccountRef(%q) did not qualify the email", email)
		}
		if got := shortenServiceAccountRef(expanded); got != email {
			t.Errorf("round trip of %q gave %q via %q", email, got, expanded)
		}
	}
}

// An already-qualified value is left alone, so a forma may name a service
// account outside the target project explicitly.
func TestExpandServiceAccountRefIsIdempotent(t *testing.T) {
	full := "projects/other/serviceAccounts/sa@other.iam.gserviceaccount.com"
	if got := expandServiceAccountRef(full, "development-477117"); got != full {
		t.Errorf("expandServiceAccountRef(%q) = %q, want unchanged", full, got)
	}
	if got := expandServiceAccountRef("", "development-477117"); got != "" {
		t.Errorf("expandServiceAccountRef(\"\") = %q, want empty", got)
	}
}

// resourceName must never reach the API: Cloud Build authorizes a PATCH against
// the body's resourceName rather than the URL's, so a stored one turns an
// in-project update into a 403 on another project's path.
func TestBuildTriggerRequestTransformerStripsOutputOnlyFields(t *testing.T) {
	for _, op := range []resource.Operation{resource.OperationCreate, resource.OperationUpdate} {
		ctx := base.TransformContext{Project: "development-477117", Operation: op}
		got, err := buildTriggerRequestTransformer(map[string]interface{}{
			"name":           "formae-test-cb-trigger",
			"id":             "9cf38638-b297-42d5-acc3-e0cf85214cb5",
			"createTime":     "2026-09-03T15:08:04.457201005Z",
			"resourceName":   "projects/p/locations/global/triggers/9cf38638",
			"serviceAccount": "formae-tester@development-477117.iam.gserviceaccount.com",
		}, ctx)
		if err != nil {
			t.Fatalf("%s: %v", op, err)
		}
		for _, f := range buildTriggerOutputOnly {
			if _, present := got[f]; present {
				t.Errorf("%s: %q survived into the request body", op, f)
			}
		}
		// name is the client-chosen id on create and a harmless identity on
		// update, so it stays either way.
		if got["name"] != "formae-test-cb-trigger" {
			t.Errorf("%s: name = %v, want it kept", op, got["name"])
		}
		want := "projects/development-477117/serviceAccounts/formae-tester@development-477117.iam.gserviceaccount.com"
		if got["serviceAccount"] != want {
			t.Errorf("%s: serviceAccount = %v, want %q", op, got["serviceAccount"], want)
		}
	}
}

// Cloud Build drops "disabled": false and reduces
// {"approvalRequired": false} to {}. Both come back as drift unless restored.
func TestBuildTriggerResponseTransformerRestoresDroppedBooleans(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]interface{}
		want map[string]interface{}
	}{
		{
			name: "the dropped disabled=false is restored",
			in: map[string]interface{}{
				"name":           "formae-test-cb-trigger",
				"serviceAccount": "projects/p/serviceAccounts/sa@p.iam.gserviceaccount.com",
			},
			want: map[string]interface{}{
				"name":           "formae-test-cb-trigger",
				"serviceAccount": "sa@p.iam.gserviceaccount.com",
				"disabled":       false,
			},
		},
		{
			name: "a trigger that really is disabled keeps it",
			in: map[string]interface{}{
				"name":     "formae-test-cb-trigger",
				"disabled": true,
			},
			want: map[string]interface{}{
				"name":     "formae-test-cb-trigger",
				"disabled": true,
			},
		},
		{
			name: "an emptied approvalConfig regains its only field",
			in: map[string]interface{}{
				"name":           "formae-test-cb-trigger",
				"disabled":       true,
				"approvalConfig": map[string]interface{}{},
			},
			want: map[string]interface{}{
				"name":     "formae-test-cb-trigger",
				"disabled": true,
				"approvalConfig": map[string]interface{}{
					"approvalRequired": false,
				},
			},
		},
		{
			name: "an absent approvalConfig is not invented",
			in: map[string]interface{}{
				"name":     "formae-test-cb-trigger",
				"disabled": true,
			},
			want: map[string]interface{}{
				"name":     "formae-test-cb-trigger",
				"disabled": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildTriggerResponseTransformer(tt.in, base.TransformContext{})
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

// The native id is keyed on the user-assigned name, never on the uuid path the
// API reports in resourceName - a uuid could not be rebuilt from a forma, and a
// leaked trigger could not be found by a name-prefix sweep.
func TestExtractCloudBuildNativeID(t *testing.T) {
	response := map[string]interface{}{
		"name":         "formae-test-cb-trigger",
		"id":           "9cf38638-b297-42d5-acc3-e0cf85214cb5",
		"resourceName": "projects/development-477117/locations/global/triggers/9cf38638-b297-42d5-acc3-e0cf85214cb5",
	}
	want := "projects/development-477117/triggers/formae-test-cb-trigger"

	// Create: the declared name is already in context.
	ctx := base.PathContext{
		Project:      "development-477117",
		ResourceType: "triggers",
		ResourceName: "formae-test-cb-trigger",
	}
	if got := extractCloudBuildNativeID(response, ctx); got != want {
		t.Errorf("with declared name: got %q, want %q", got, want)
	}

	// List: a discovered item has no declared name, so the response's own
	// short "name" is what identifies it.
	listCtx := base.PathContext{Project: "development-477117", ResourceType: "triggers"}
	if got := extractCloudBuildNativeID(response, listCtx); got != want {
		t.Errorf("from a list item: got %q, want %q", got, want)
	}

	// A native id must parse back into the context a read needs.
	parsed, err := base.ParseNativeID(CloudBuildNativeID, want)
	if err != nil {
		t.Fatalf("ParseNativeID: %v", err)
	}
	if parsed.Project != "development-477117" ||
		parsed.ResourceType != "triggers" ||
		parsed.ResourceName != "formae-test-cb-trigger" {
		t.Errorf("ParseNativeID(%q) = %+v", want, parsed)
	}
	if got := cloudBuildPathBuilder(parsed); got != "/projects/development-477117/triggers/formae-test-cb-trigger" {
		t.Errorf("path built from the parsed id = %q", got)
	}
}
