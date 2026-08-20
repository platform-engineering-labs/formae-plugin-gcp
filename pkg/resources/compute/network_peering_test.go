// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"reflect"
	"testing"
)

const peeringPath = "projects/dev-1/global/networks/net-a/peerings/link-b"

// A peering is identified by (owning network, name); both must survive the
// native ID round-trip or Read cannot find it again.
func TestPeeringNativeIDRoundTrip(t *testing.T) {
	if got := buildPeeringNativeID("dev-1", "net-a", "link-b"); got != peeringPath {
		t.Fatalf("build: %q", got)
	}
	project, network, name, err := parsePeeringNativeID(peeringPath)
	if err != nil {
		t.Fatal(err)
	}
	if project != "dev-1" || network != "net-a" || name != "link-b" {
		t.Errorf("parse: %q %q %q", project, network, name)
	}
}

func TestParsePeeringNativeIDRejectsOtherShapes(t *testing.T) {
	for _, bad := range []string{
		"projects/dev-1/global/networks/net-a",                    // the network itself
		"projects/dev-1/regions/r/networks/net-a/peerings/link-b", // regional shape
		"projects/dev-1/global/firewallPolicies/p/rules/1000",     // a firewall rule id
		"",
	} {
		if _, _, _, err := parsePeeringNativeID(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

// addPeering wraps the peering in a "networkPeering" object, calls the far side
// "network", and rejects the owning network as an unknown field.
func TestPeeringBodyWrapsAndRenames(t *testing.T) {
	body := peeringBody(map[string]interface{}{
		"name":                 "link-b",
		"network":              "net-a",
		"peerNetwork":          "projects/dev-1/global/networks/net-b",
		"exchangeSubnetRoutes": true,
		"state":                "INACTIVE",
		"stateDetails":         "waiting",
	})
	wrapped, ok := body["networkPeering"].(map[string]interface{})
	if !ok {
		t.Fatalf("not wrapped in networkPeering: %#v", body)
	}
	if wrapped["network"] != "projects/dev-1/global/networks/net-b" {
		t.Errorf("far side not mapped onto network: %#v", wrapped)
	}
	if _, ok := wrapped["peerNetwork"]; ok {
		t.Errorf("schema field must not reach the body: %#v", wrapped)
	}
	if wrapped["name"] != "link-b" || wrapped["exchangeSubnetRoutes"] != true {
		t.Errorf("real fields must survive: %#v", wrapped)
	}
	// Read-only fields would be rejected by the API.
	for _, k := range []string{"state", "stateDetails"} {
		if _, ok := wrapped[k]; ok {
			t.Errorf("read-only %q must be stripped: %#v", k, wrapped)
		}
	}
	// The owning network is a path component only.
	if _, ok := wrapped["networkPeering"]; ok {
		t.Errorf("unexpected nesting: %#v", wrapped)
	}
	if got, want := len(wrapped), 3; got != want {
		t.Errorf("unexpected body keys %#v", reflect.ValueOf(wrapped).MapKeys())
	}
}
