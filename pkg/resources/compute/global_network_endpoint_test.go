// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import "testing"

const endpointPath = "projects/dev-1/global/networkEndpointGroups/neg-a/networkEndpoints/origin.example.com|443"

// The composite id must round-trip: Read and Delete both rebuild the endpoint
// from it, so an off-by-one here breaks teardown while create still looks fine.
func TestEndpointNativeIDRoundTrip(t *testing.T) {
	if got := buildEndpointNativeID("dev-1", "neg-a", "origin.example.com", 443); got != endpointPath {
		t.Fatalf("build: %q", got)
	}
	project, group, host, port, err := parseEndpointNativeID(endpointPath)
	if err != nil {
		t.Fatal(err)
	}
	if project != "dev-1" || group != "neg-a" || host != "origin.example.com" || port != 443 {
		t.Errorf("parse: %q %q %q %d", project, group, host, port)
	}
}

// An IPv6 literal contains colons and an FQDN contains dots, which is why the
// key is pipe-separated.
func TestEndpointNativeIDHandlesIPv6(t *testing.T) {
	id := buildEndpointNativeID("dev-1", "neg-a", "2001:db8::1", 8443)
	_, _, host, port, err := parseEndpointNativeID(id)
	if err != nil {
		t.Fatal(err)
	}
	if host != "2001:db8::1" || port != 8443 {
		t.Errorf("got %q %d", host, port)
	}
}

func TestParseEndpointNativeIDRejectsOtherShapes(t *testing.T) {
	for _, bad := range []string{
		"projects/dev-1/global/networkEndpointGroups/neg-a",                        // the group itself
		"projects/dev-1/global/networkEndpointGroups/neg-a/networkEndpoints/nokey", // no |port
		"projects/dev-1/global/networkEndpointGroups/neg-a/networkEndpoints/h|abc", // non-numeric port
		"projects/dev-1/global/networks/n/peerings/p",                              // a peering id
		"",
	} {
		if _, _, _, _, err := parseEndpointNativeID(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

// A global NEG endpoint is addressed by fqdn or ipAddress, never both.
func TestEndpointOfRejectsAmbiguousHost(t *testing.T) {
	if _, _, _, err := endpointOf(map[string]interface{}{
		"fqdn": "a.example.com", "ipAddress": "10.0.0.1", "port": float64(443),
	}); err == nil {
		t.Error("expected an error when both fqdn and ipAddress are set")
	}
	if _, _, _, err := endpointOf(map[string]interface{}{"port": float64(443)}); err == nil {
		t.Error("expected an error when neither fqdn nor ipAddress is set")
	}
}

func TestEndpointOfBuildsFqdnEndpoint(t *testing.T) {
	endpoint, host, port, err := endpointOf(map[string]interface{}{
		"fqdn": "origin.example.com", "port": float64(443),
		"networkEndpointGroup": "neg-a", // must not leak into the endpoint
	})
	if err != nil {
		t.Fatal(err)
	}
	if host != "origin.example.com" || port != 443 {
		t.Errorf("host/port: %q %d", host, port)
	}
	if endpoint["fqdn"] != "origin.example.com" || endpoint["port"] != 443 {
		t.Errorf("endpoint: %#v", endpoint)
	}
	if _, ok := endpoint["networkEndpointGroup"]; ok {
		t.Errorf("group must not reach the endpoint body: %#v", endpoint)
	}
}
