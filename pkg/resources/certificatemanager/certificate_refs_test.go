//go:build unit

// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package certificatemanager

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// A forma names another resource by its short id, because that is all a
// resolvable can yield. The request has to carry a full path.
func TestCertificateRequestExpandsAuthorizations(t *testing.T) {
	body := map[string]interface{}{
		"name":               "cert1",
		"managedCertificate": map[string]interface{}{"dnsAuthorizations": []interface{}{"auth1"}},
	}
	out, err := certificateRequestTransformer(body, base.TransformContext{Project: "p"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	managed, ok := out["managed"].(map[string]interface{})
	if !ok {
		t.Fatalf("managedCertificate was not renamed to managed: %v", out)
	}
	got := managed["dnsAuthorizations"].([]interface{})[0]
	want := "projects/p/locations/global/dnsAuthorizations/auth1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// And the answer has to come back in the form the forma used, or an immutable
// field disagrees with itself and every re-apply plans a replacement.
func TestCertificateResponseShortensAuthorizations(t *testing.T) {
	resp := map[string]interface{}{
		"name": "projects/p/locations/global/certificates/cert1",
		"managed": map[string]interface{}{
			// as reported: project number, not project id
			"dnsAuthorizations": []interface{}{"projects/989754770009/locations/global/dnsAuthorizations/auth1"},
			"state":             "PROVISIONING",
		},
	}
	out := certificateResponseTransformer.Transform(resp, base.TransformContext{Project: "p"})
	managed, ok := out["managedCertificate"].(map[string]interface{})
	if !ok {
		t.Fatalf("managed was not renamed back: %v", out)
	}
	if got := managed["dnsAuthorizations"].([]interface{})[0]; got != "auth1" {
		t.Errorf("authorization: got %q, want %q", got, "auth1")
	}
	if _, present := managed["state"]; present {
		t.Error("output-only state survived; Verify will reject it as undeclared")
	}
}

// The round trip is the identity on the declared form. If it is not, the field
// drifts by construction.
func TestCertificateAuthorizationRoundTrips(t *testing.T) {
	body := map[string]interface{}{
		"managedCertificate": map[string]interface{}{"dnsAuthorizations": []interface{}{"auth1"}},
	}
	req, err := certificateRequestTransformer(body, base.TransformContext{Project: "p"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := certificateResponseTransformer.Transform(req, base.TransformContext{Project: "p"})
	managed := out["managedCertificate"].(map[string]interface{})
	if got := managed["dnsAuthorizations"].([]interface{})[0]; got != "auth1" {
		t.Errorf("round trip changed the value: got %q, want %q", got, "auth1")
	}
}

// A List with no parent must still address a collection that exists, or the
// nested type is never discovered.
func TestPathBuilderWildcardsAParentlessList(t *testing.T) {
	got := certificateManagerPathBuilder(base.PathContext{
		Project:      "p",
		ResourceType: "certificateMapEntries",
	})
	want := "/projects/p/locations/global/certificateMaps/-/certificateMapEntries"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// With a parent it addresses that parent, not the wildcard.
func TestPathBuilderUsesTheRealParentWhenGiven(t *testing.T) {
	got := certificateManagerPathBuilder(base.PathContext{
		Project:        "p",
		ParentType:     "certificateMaps",
		ParentResource: "map1",
		ResourceType:   "certificateMapEntries",
		ResourceName:   "e1",
	})
	want := "/projects/p/locations/global/certificateMaps/map1/certificateMapEntries/e1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The map is a path component, so it must not reach the body on create either.
func TestEntryRequestDropsTheMapFromTheBody(t *testing.T) {
	out, err := certificateMapEntryRequestTransformer(
		map[string]interface{}{"name": "e1", "certificateMap": "map1", "certificates": []interface{}{"cert1"}},
		base.TransformContext{Project: "p", Operation: "Create"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, present := out["certificateMap"]; present {
		t.Error("certificateMap reached the body; the API rejects it as unknown")
	}
	if got := out["certificates"].([]interface{})[0]; got != "projects/p/locations/global/certificates/cert1" {
		t.Errorf("certificate was not expanded: %v", got)
	}
}
