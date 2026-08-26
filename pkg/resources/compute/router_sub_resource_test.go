// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import "testing"

const routePolicyPath = "projects/dev-1/regions/europe-central2/routers/rt-1/routePolicies/pol-a"

// A policy is addressed by (region, router, name); all three must survive, since
// every verb URL is rebuilt from the id.
func TestRoutePolicyNativeIDRoundTrip(t *testing.T) {
	if got := routePolicyKind.nativeID("dev-1", "europe-central2", "rt-1", "pol-a"); got != routePolicyPath {
		t.Fatalf("build: %q", got)
	}
	project, region, router, name, err := routePolicyKind.parseNativeID(routePolicyPath)
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
		if _, _, _, _, err := routePolicyKind.parseNativeID(bad); err == nil {
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
	body := routerSubBody(map[string]interface{}{
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

// The two kinds share one implementation, so each must reject the other's ids —
// a named set addressed through the route-policy verbs would simply not be
// found, which would read as drift rather than as a bug.
func TestRouterSubKindsRejectEachOthersIDs(t *testing.T) {
	const namedSetPath = "projects/dev-1/regions/europe-central2/routers/rt-1/namedSets/set-a"
	if got := namedSetKind.nativeID("dev-1", "europe-central2", "rt-1", "set-a"); got != namedSetPath {
		t.Fatalf("build: %q", got)
	}
	project, region, router, name, err := namedSetKind.parseNativeID(namedSetPath)
	if err != nil {
		t.Fatal(err)
	}
	if project != "dev-1" || region != "europe-central2" || router != "rt-1" || name != "set-a" {
		t.Errorf("parse: %q %q %q %q", project, region, router, name)
	}
	if _, _, _, _, err := namedSetKind.parseNativeID(routePolicyPath); err == nil {
		t.Error("named set kind accepted a route policy ID")
	}
	if _, _, _, _, err := routePolicyKind.parseNativeID(namedSetPath); err == nil {
		t.Error("route policy kind accepted a named set ID")
	}
}

// The list verb pluralises its noun, so the four verbs cannot be derived from
// one string — a wrong guess here is a 404 at runtime.
func TestRouterSubKindVerbs(t *testing.T) {
	for _, k := range []struct {
		kind                                   routerSubKind
		update, get, del, list, param, segment string
	}{
		{routePolicyKind, "updateRoutePolicy", "getRoutePolicy", "deleteRoutePolicy", "listRoutePolicies", "policy", "routePolicies"},
		{namedSetKind, "updateNamedSet", "getNamedSet", "deleteNamedSet", "listNamedSets", "namedSet", "namedSets"},
	} {
		if k.kind.updateVerb != k.update || k.kind.getVerb != k.get ||
			k.kind.deleteVerb != k.del || k.kind.listVerb != k.list ||
			k.kind.queryParam != k.param || k.kind.segment != k.segment {
			t.Errorf("verbs wrong for %s: %+v", k.kind.label, k.kind)
		}
	}
}

// A named set carries elements where a route policy carries terms; both must
// pass through the body untouched, and neither may leak the path components.
func TestNamedSetBodyKeepsElements(t *testing.T) {
	body := routerSubBody(map[string]interface{}{
		"router":      "rt-1",
		"region":      "europe-central2",
		"fingerprint": "stale==",
		"name":        "set-a",
		"type":        "NAMED_SET_TYPE_PREFIX",
		"elements":    []interface{}{map[string]interface{}{"expression": "'10.0.0.0/8'"}},
	})
	for _, k := range []string{"router", "region", "fingerprint"} {
		if _, ok := body[k]; ok {
			t.Errorf("%s must not reach the body: %#v", k, body)
		}
	}
	elements, ok := body["elements"].([]interface{})
	if !ok || len(elements) != 1 {
		t.Fatalf("elements: %#v", body["elements"])
	}
	if elements[0].(map[string]interface{})["expression"] != "'10.0.0.0/8'" {
		t.Errorf("element lost: %#v", elements[0])
	}
}
