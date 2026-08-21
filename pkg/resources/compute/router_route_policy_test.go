// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import "testing"

const routePolicyPath = "projects/dev-1/regions/europe-central2/routers/rt-1/routePolicies/pol-a"

// A policy is addressed by (region, router, name); all three must survive, since
// every verb URL is rebuilt from the id.
func TestRoutePolicyNativeIDRoundTrip(t *testing.T) {
	if got := buildRoutePolicyNativeID("dev-1", "europe-central2", "rt-1", "pol-a"); got != routePolicyPath {
		t.Fatalf("build: %q", got)
	}
	project, region, router, name, err := parseRoutePolicyNativeID(routePolicyPath)
	if err != nil {
		t.Fatal(err)
	}
	if project != "dev-1" || region != "europe-central2" || router != "rt-1" || name != "pol-a" {
		t.Errorf("parse: %q %q %q %q", project, region, router, name)
	}
}

func TestParseRoutePolicyNativeIDRejectsOtherShapes(t *testing.T) {
	for _, bad := range []string{
		"projects/dev-1/regions/europe-central2/routers/rt-1",                 // the router itself
		"projects/dev-1/regions/europe-central2/routers/rt-1/routePolicies/",  // no name
		"projects/dev-1/regions//routers/rt-1/routePolicies/pol-a",            // empty region
		"projects/dev-1/global/routers/rt-1/routePolicies/pol-a",              // global shape
		"projects/dev-1/regions/europe-central2/routers/rt-1/namedSets/pol-a", // wrong sub-collection
		"",
	} {
		if _, _, _, _, err := parseRoutePolicyNativeID(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

// The router and region address the URL, and the fingerprint must come from a
// fresh read rather than the declared forma — a stale one is rejected.
func TestRoutePolicyBodyDropsPathAndFingerprint(t *testing.T) {
	terms := []interface{}{
		map[string]interface{}{
			"priority": float64(1),
			"match":    map[string]interface{}{"expression": "destination == '10.0.0.0/8'"},
			"actions":  []interface{}{map[string]interface{}{"expression": "accept()"}},
		},
	}
	body := routePolicyBody(map[string]interface{}{
		"router":      "rt-1",
		"region":      "europe-central2",
		"fingerprint": "stale==",
		"name":        "pol-a",
		"type":        "ROUTE_POLICY_TYPE_IMPORT",
		"terms":       terms,
	})
	for _, k := range []string{"router", "region", "fingerprint"} {
		if _, ok := body[k]; ok {
			t.Errorf("%s must not reach the body: %#v", k, body)
		}
	}
	if body["name"] != "pol-a" || body["type"] != "ROUTE_POLICY_TYPE_IMPORT" {
		t.Errorf("policy fields must survive: %#v", body)
	}
	// The terms must pass through untouched — they carry the whole BGP rule.
	got, ok := body["terms"].([]interface{})
	if !ok || len(got) != 1 {
		t.Fatalf("terms: %#v", body["terms"])
	}
	term := got[0].(map[string]interface{})
	if term["match"].(map[string]interface{})["expression"] != "destination == '10.0.0.0/8'" {
		t.Errorf("term match lost: %#v", term)
	}
}
