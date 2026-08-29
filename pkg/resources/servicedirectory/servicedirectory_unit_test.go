// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package servicedirectory

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// The three levels round-trip through the native ID, including the endpoint's
// two-parent form carried as "{namespace}/{service}".
func TestNativeIDRoundTrip(t *testing.T) {
	for _, nativeID := range []string{
		"projects/p/locations/eu/namespaces/ns",
		"projects/p/locations/eu/namespaces/ns/services/svc",
		"projects/p/locations/eu/namespaces/ns/services/svc/endpoints/ep",
	} {
		ctx, err := parseServiceDirectoryNativeID(nativeID)
		if err != nil {
			t.Fatalf("parse(%q) failed: %v", nativeID, err)
		}
		if got := "/" + nativeID; serviceDirectoryPathBuilder(ctx) != got {
			t.Errorf("path for %q = %q, want %q", nativeID, serviceDirectoryPathBuilder(ctx), got)
		}
	}
}

func TestParseNativeIDRejectsOtherShapes(t *testing.T) {
	for _, bad := range []string{
		"projects/p/locations/eu",
		"projects/p/locations/eu/namespaces/ns/services",
		"namespaces/ns/services/svc",
		"projects/p/regions/eu/namespaces/ns",
	} {
		if _, err := parseServiceDirectoryNativeID(bad); err == nil {
			t.Errorf("parse(%q) should have failed", bad)
		}
	}
}

// The response reports the full path as "name"; a forma declares the short id
// plus the parents it hangs off, so each piece has to be put back.
func TestResponseTransformerSplitsThePath(t *testing.T) {
	out := responseTransformer(map[string]interface{}{
		"name": "projects/p/locations/eu/namespaces/ns/services/svc/endpoints/ep",
		"port": 8080,
	}, base.TransformContext{Project: "p"})

	for field, want := range map[string]string{
		"name":      "ep",
		"service":   "svc",
		"namespace": "ns",
		"project":   "p",
	} {
		if got, _ := out[field].(string); got != want {
			t.Errorf("%s = %q, want %q", field, got, want)
		}
	}
	if out["port"] != 8080 {
		t.Errorf("port was dropped: %v", out["port"])
	}
}

// The id travels as a create-time query parameter and the rest of the address
// is in the URL, so a body carrying them is rejected.
func TestRequestTransformerDropsAddressingFields(t *testing.T) {
	body, err := requestTransformer(map[string]interface{}{
		"name":        "ep",
		"project":     "p",
		"namespace":   "ns",
		"service":     "svc",
		"address":     "192.0.2.10",
		"annotations": map[string]interface{}{"a": "b"},
	}, base.TransformContext{})
	if err != nil {
		t.Fatalf("transform failed: %v", err)
	}
	for _, dropped := range []string{"name", "project", "namespace", "service"} {
		if _, ok := body[dropped]; ok {
			t.Errorf("%s should not be in the body", dropped)
		}
	}
	if body["address"] != "192.0.2.10" || body["annotations"] == nil {
		t.Errorf("descriptive fields were dropped: %v", body)
	}
}
