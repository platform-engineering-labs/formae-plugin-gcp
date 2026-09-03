//go:build unit

// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package networkservices

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// The whole point of the transformer: the API drops a false "enable" out of its
// JSON, so without putting it back a policy that turns the drain off disagrees
// with state on every reconcile and the drift never settles.
func TestServiceLbPolicyRestoresOmittedEnable(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   map[string]interface{}
		want interface{} // expected autoCapacityDrain, nil means absent
	}{
		{
			name: "omitted enable is restored as false",
			in:   map[string]interface{}{"autoCapacityDrain": map[string]interface{}{}},
			want: map[string]interface{}{"enable": false},
		},
		{
			name: "explicit false is left alone",
			in:   map[string]interface{}{"autoCapacityDrain": map[string]interface{}{"enable": false}},
			want: map[string]interface{}{"enable": false},
		},
		{
			name: "true is left alone",
			in:   map[string]interface{}{"autoCapacityDrain": map[string]interface{}{"enable": true}},
			want: map[string]interface{}{"enable": true},
		},
		{
			// Adding a block the forma never declared would be the same drift
			// pointing the other way.
			name: "absent block stays absent",
			in:   map[string]interface{}{"description": "d"},
			want: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := serviceLbPolicyResponseTransformer(tc.in, base.TransformContext{})
			drain, present := got["autoCapacityDrain"]
			if tc.want == nil {
				if present {
					t.Fatalf("autoCapacityDrain should be absent, got %v", drain)
				}
				return
			}
			if !present {
				t.Fatal("autoCapacityDrain missing")
			}
			m, ok := drain.(map[string]interface{})
			if !ok {
				t.Fatalf("autoCapacityDrain is %T, want map", drain)
			}
			want := tc.want.(map[string]interface{})
			if len(m) != len(want) {
				t.Fatalf("got %v, want %v", m, want)
			}
			for k, v := range want {
				if m[k] != v {
					t.Errorf("%s: got %v, want %v", k, m[k], v)
				}
			}
		})
	}
}

// A declared false has to survive the round trip, or the field drifts by
// construction: what a forma sends is what a read must report back.
func TestServiceLbPolicyEnableRoundTrips(t *testing.T) {
	// What the forma declares.
	declared := map[string]interface{}{"enable": false}
	// What the API stores it as and hands back.
	apiSaid := map[string]interface{}{"autoCapacityDrain": map[string]interface{}{}}

	out := serviceLbPolicyResponseTransformer(apiSaid, base.TransformContext{})
	got := out["autoCapacityDrain"].(map[string]interface{})
	if got["enable"] != declared["enable"] {
		t.Errorf("round trip changed the value: got %v, want %v", got["enable"], declared["enable"])
	}
}

// The transformer must not write through to the response map it was handed:
// that map can be shared with the raw decoded body, and a caller that reads it
// afterwards should see what the API actually said.
func TestServiceLbPolicyDoesNotMutateInputDrain(t *testing.T) {
	drain := map[string]interface{}{}
	serviceLbPolicyResponseTransformer(
		map[string]interface{}{"autoCapacityDrain": drain}, base.TransformContext{})
	if _, present := drain["enable"]; present {
		t.Error("transformer mutated the caller's nested map")
	}
}

// It still has to do the job every other resource in this API needs: the API
// reports "name" as a full path and a forma declares the short id.
func TestServiceLbPolicyShortensName(t *testing.T) {
	out := serviceLbPolicyResponseTransformer(map[string]interface{}{
		"name": "projects/p/locations/global/serviceLbPolicies/my-policy",
	}, base.TransformContext{})
	if out["name"] != "my-policy" {
		t.Errorf("got %q, want %q", out["name"], "my-policy")
	}
}
