// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"reflect"
	"testing"
)

const routerInterfacePath = "projects/dev-1/regions/europe-central2/routers/rt-1/interfaces/if-a"

func TestRouterInterfaceNativeIDRoundTrip(t *testing.T) {
	if got := buildRouterInterfaceNativeID("dev-1", "europe-central2", "rt-1", "if-a"); got != routerInterfacePath {
		t.Fatalf("build: %q", got)
	}
	project, region, router, name, err := parseRouterInterfaceNativeID(routerInterfacePath)
	if err != nil {
		t.Fatal(err)
	}
	if project != "dev-1" || region != "europe-central2" || router != "rt-1" || name != "if-a" {
		t.Errorf("parse: %q %q %q %q", project, region, router, name)
	}
	for _, bad := range []string{
		"projects/dev-1/regions/europe-central2/routers/rt-1",             // the router
		"projects/dev-1/regions/europe-central2/routers/rt-1/nats/nat-a",  // a NAT
		"projects/dev-1/regions/europe-central2/routers/rt-1/interfaces/", // no name
		"projects/dev-1/global/routers/rt-1/interfaces/if-a",              // global shape
		"",
	} {
		if _, _, _, _, err := parseRouterInterfaceNativeID(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

// routers.patch replaces the whole interfaces array, so anything this drops is
// dropped from the router — sibling interfaces must always survive.
func TestMergeRouterInterfacePreservesSiblings(t *testing.T) {
	existing := []interface{}{
		map[string]interface{}{"name": "if-vpn", "linkedVpnTunnel": "tunnel-1"},
		map[string]interface{}{"name": "if-a", "privateIpAddress": "10.44.0.5"},
	}

	// Overwrite in place, leaving the sibling first.
	updated := mergeRouterInterface(existing, "if-a",
		map[string]interface{}{"name": "if-a", "privateIpAddress": "10.44.0.6"}, false)
	if len(updated) != 2 {
		t.Fatalf("length changed: %#v", updated)
	}
	if got := updated[0].(map[string]interface{})["name"]; got != "if-vpn" {
		t.Errorf("sibling moved or lost: %#v", updated)
	}
	if got := updated[1].(map[string]interface{})["privateIpAddress"]; got != "10.44.0.6" {
		t.Errorf("overwrite failed: %#v", updated)
	}

	// Add a new one.
	added := mergeRouterInterface(existing, "if-b",
		map[string]interface{}{"name": "if-b"}, false)
	if len(added) != 3 || added[2].(map[string]interface{})["name"] != "if-b" {
		t.Errorf("add: %#v", added)
	}

	// Remove exactly one, keeping the VPN interface someone else owns.
	removed := mergeRouterInterface(existing, "if-a", nil, true)
	if len(removed) != 1 || removed[0].(map[string]interface{})["name"] != "if-vpn" {
		t.Errorf("remove: %#v", removed)
	}

	// Removing an absent interface is a no-op, not a wipe.
	if got := mergeRouterInterface(existing, "nope", nil, true); len(got) != 2 {
		t.Errorf("removing an absent interface changed the list: %#v", got)
	}
	// An empty router accepts the first interface.
	if got := mergeRouterInterface(nil, "if-a", map[string]interface{}{"name": "if-a"}, false); len(got) != 1 {
		t.Errorf("empty router: %#v", got)
	}
	// Junk entries must not panic or survive as junk.
	junk := []interface{}{"not-a-map", map[string]interface{}{"name": "real"}}
	if got := mergeRouterInterface(junk, "new", map[string]interface{}{"name": "new"}, false); len(got) != 2 {
		t.Errorf("junk handling: %#v", got)
	}
}

func TestRouterInterfaceBodyDropsPathProps(t *testing.T) {
	body := routerInterfaceBody(map[string]interface{}{
		"router":           "rt-1",
		"region":           "europe-central2",
		"name":             "if-a",
		"subnetwork":       "projects/dev-1/regions/europe-central2/subnetworks/sub-1",
		"privateIpAddress": "10.44.0.5",
	})
	for _, k := range []string{"router", "region"} {
		if _, ok := body[k]; ok {
			t.Errorf("%s must not reach the body: %#v", k, body)
		}
	}
	want := map[string]interface{}{
		"name":             "if-a",
		"subnetwork":       "projects/dev-1/regions/europe-central2/subnetworks/sub-1",
		"privateIpAddress": "10.44.0.5",
	}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("body: %#v", body)
	}
}

func TestFindRouterInterface(t *testing.T) {
	router := map[string]interface{}{
		"interfaces": []interface{}{
			map[string]interface{}{"name": "if-a", "ipRange": "10.44.0.5/24"},
		},
	}
	if got := findRouterInterface(router, "if-a"); got == nil || got["ipRange"] != "10.44.0.5/24" {
		t.Errorf("lookup: %#v", got)
	}
	if got := findRouterInterface(router, "missing"); got != nil {
		t.Errorf("absent interface reported: %#v", got)
	}
	// A router with no interfaces at all must not panic.
	if got := findRouterInterface(map[string]interface{}{"name": "rt-1"}, "if-a"); got != nil {
		t.Errorf("empty router reported an interface: %#v", got)
	}
}
