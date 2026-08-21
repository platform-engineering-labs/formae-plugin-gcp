// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import "testing"

const associationPath = "projects/dev-1/global/firewallPolicies/pol-a/associations/assoc-1"

// An association is addressed by (policy, name), so both must survive the
// round-trip — Read and Delete rebuild the verb URL from the id.
func TestAssociationNativeIDRoundTrip(t *testing.T) {
	if got := buildAssociationNativeID("dev-1", "pol-a", "assoc-1"); got != associationPath {
		t.Fatalf("build: %q", got)
	}
	project, policy, name, err := parseAssociationNativeID(associationPath)
	if err != nil {
		t.Fatal(err)
	}
	if project != "dev-1" || policy != "pol-a" || name != "assoc-1" {
		t.Errorf("parse: %q %q %q", project, policy, name)
	}
}

func TestParseAssociationNativeIDRejectsOtherShapes(t *testing.T) {
	for _, bad := range []string{
		"projects/dev-1/global/firewallPolicies/pol-a",                   // the policy itself
		"projects/dev-1/global/firewallPolicies/pol-a/rules/1000",        // a rule
		"projects/dev-1/global/firewallPolicies/pol-a/associations/",     // no name
		"projects/dev-1/regions/r/firewallPolicies/pol-a/associations/a", // regional shape
		"projects/dev-1/global/networks/n/associations/a",                // wrong collection
		"",
	} {
		if _, _, _, err := parseAssociationNativeID(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

// The attach verb takes only the association's name and its target; the owning
// policy addresses the URL and the API rejects it as a body field.
func TestAssociationBodyKeepsOnlyAttachFields(t *testing.T) {
	body := associationBody(map[string]interface{}{
		"firewallPolicy":   "pol-a",
		"name":             "assoc-1",
		"attachmentTarget": "projects/dev-1/global/networks/n-1",
		"shortName":        "ignored",
	})
	if _, ok := body["firewallPolicy"]; ok {
		t.Errorf("policy must not reach the body: %#v", body)
	}
	if _, ok := body["shortName"]; ok {
		t.Errorf("unknown fields must not reach the body: %#v", body)
	}
	if body["name"] != "assoc-1" || body["attachmentTarget"] != "projects/dev-1/global/networks/n-1" {
		t.Errorf("attach fields must survive: %#v", body)
	}
	// A missing target must not be sent as an empty string — the API would
	// reject it with a less obvious error than the plugin's own check.
	if _, ok := associationBody(map[string]interface{}{"name": "a"})["attachmentTarget"]; ok {
		t.Error("empty attachmentTarget should be omitted")
	}
}
