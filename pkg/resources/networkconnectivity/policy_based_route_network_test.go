//go:build unit

// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package networkconnectivity

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// A reference to a network resolves to a self link, and a policy-based route
// rejects one outright. The request has to carry the path form.
func TestPolicyBasedRouteRequestCutsSelfLink(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"https://www.googleapis.com/compute/v1/projects/p/global/networks/n", "projects/p/global/networks/n"},
		{"projects/p/global/networks/n", "projects/p/global/networks/n"},
		{"", ""},
	} {
		body := map[string]interface{}{"network": tc.in}
		got, err := policyBasedRouteRequestTransformer(body, base.TransformContext{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["network"] != tc.want {
			t.Errorf("network %q: got %q, want %q", tc.in, got["network"], tc.want)
		}
	}
}

// The API reports back the path form, so without the mirror the forma holds a
// self link, state holds a path, and an immutable field disagrees on every
// re-apply - which plans a replacement of the route already in place.
func TestPolicyBasedRouteResponseExpandsToSelfLink(t *testing.T) {
	out := policyBasedRouteResponseTransformer(
		map[string]interface{}{"network": "projects/p/global/networks/n"}, base.TransformContext{})
	want := "https://www.googleapis.com/compute/v1/projects/p/global/networks/n"
	if out["network"] != want {
		t.Errorf("got %q, want %q", out["network"], want)
	}
}

// A round trip has to be the identity, or the field drifts by construction.
func TestPolicyBasedRouteNetworkRoundTrips(t *testing.T) {
	const declared = "https://www.googleapis.com/compute/v1/projects/p/global/networks/n"
	req, err := policyBasedRouteRequestTransformer(map[string]interface{}{"network": declared}, base.TransformContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := policyBasedRouteResponseTransformer(req, base.TransformContext{})
	if out["network"] != declared {
		t.Errorf("round trip changed the value: got %q, want %q", out["network"], declared)
	}
}
