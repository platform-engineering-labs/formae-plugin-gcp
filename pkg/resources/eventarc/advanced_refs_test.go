// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package eventarc

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// A forma passes resolvables, which resolve to short names; Eventarc wants full
// paths. Expand on write, shorten on read - otherwise every comparison step
// reports drift on a correct resource.
func TestEnrollmentRefsRoundTrip(t *testing.T) {
	body, err := enrollmentRequest(map[string]interface{}{
		"name":        "en-1",
		"location":    "us-east1",
		"messageBus":  "bus-1",
		"destination": "pipe-1",
		"celMatch":    "true",
	}, base.TransformContext{Project: "proj"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := body["messageBus"], "projects/proj/locations/us-east1/messageBuses/bus-1"; got != want {
		t.Errorf("messageBus = %v, want %v", got, want)
	}
	if got, want := body["destination"], "projects/proj/locations/us-east1/pipelines/pipe-1"; got != want {
		t.Errorf("destination = %v, want %v", got, want)
	}
	if _, ok := body["location"]; ok {
		t.Error("location addresses the resource in the URL and must not be a body field")
	}

	out := enrollmentResponse(map[string]interface{}{
		"name":        "projects/proj/locations/us-east1/enrollments/en-1",
		"messageBus":  body["messageBus"],
		"destination": body["destination"],
	}, base.TransformContext{})

	if got, want := out["name"], "en-1"; got != want {
		t.Errorf("name = %v, want %v", got, want)
	}
	if got, want := out["location"], "us-east1"; got != want {
		t.Errorf("location = %v, want %v", got, want)
	}
	if got, want := out["messageBus"], "bus-1"; got != want {
		t.Errorf("messageBus = %v, want %v", got, want)
	}
	if got, want := out["destination"], "pipe-1"; got != want {
		t.Errorf("destination = %v, want %v", got, want)
	}
}

// A googleApiSource's destination is a bus, not a pipeline.
func TestGoogleAPISourceRefsRoundTrip(t *testing.T) {
	body, err := googleAPISourceRequest(map[string]interface{}{
		"name":        "gas-1",
		"location":    "europe-west4",
		"destination": "bus-1",
	}, base.TransformContext{Project: "proj"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "projects/proj/locations/europe-west4/messageBuses/bus-1"
	if got := body["destination"]; got != want {
		t.Errorf("destination = %v, want %v", got, want)
	}

	out := googleAPISourceResponse(map[string]interface{}{
		"name":        "projects/proj/locations/europe-west4/googleApiSources/gas-1",
		"destination": want,
	}, base.TransformContext{})
	if got, want := out["destination"], "bus-1"; got != want {
		t.Errorf("destination = %v, want %v", got, want)
	}
	if got, want := out["name"], "gas-1"; got != want {
		t.Errorf("name = %v, want %v", got, want)
	}
}

// A forma may still write the full path by hand; expansion must not double it.
func TestExpandRefLeavesFullPathAlone(t *testing.T) {
	full := "projects/other/locations/us-west1/messageBuses/bus-9"
	body, err := googleAPISourceRequest(map[string]interface{}{
		"location":    "europe-west4",
		"destination": full,
	}, base.TransformContext{Project: "proj"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := body["destination"]; got != full {
		t.Errorf("destination = %v, want %v", got, full)
	}
}

// A bus's path must not be read as an enrollment's: locationResponseTransformer
// is keyed on the collection, so a mismatched path is left untouched.
func TestResponseIgnoresForeignCollection(t *testing.T) {
	out := enrollmentResponse(map[string]interface{}{
		"name": "projects/proj/locations/us-east1/messageBuses/bus-1",
	}, base.TransformContext{})
	if got, want := out["name"], "projects/proj/locations/us-east1/messageBuses/bus-1"; got != want {
		t.Errorf("name = %v, want %v (a bus path is not an enrollment path)", got, want)
	}
	if _, ok := out["location"]; ok {
		t.Error("location must not be inferred from a foreign collection's path")
	}
}
