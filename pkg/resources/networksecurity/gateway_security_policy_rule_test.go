// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package networksecurity

import (
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestGatewaySecurityPolicyRuleRequestTransformer(t *testing.T) {
	tests := []struct {
		name  string
		props map[string]interface{}
		op    resource.Operation
		want  map[string]interface{}
	}{
		{
			name: "create keeps the name and drops the parent",
			props: map[string]interface{}{
				"name":                  "r1",
				"gatewaySecurityPolicy": "pol",
				"enabled":               true,
				"priority":              100,
				"sessionMatcher":        `host() == "example.com"`,
				"basicProfile":          "ALLOW",
			},
			op: resource.OperationCreate,
			want: map[string]interface{}{
				"name":           "r1",
				"enabled":        true,
				"priority":       100,
				"sessionMatcher": `host() == "example.com"`,
				"basicProfile":   "ALLOW",
			},
		},
		{
			name: "update drops both, so neither enters the update mask",
			props: map[string]interface{}{
				"name":                  "r1",
				"gatewaySecurityPolicy": "pol",
				"description":           "d",
				"priority":              200,
			},
			op: resource.OperationUpdate,
			want: map[string]interface{}{
				"description": "d",
				"priority":    200,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gatewaySecurityPolicyRuleRequestTransformer.Transform(
				tt.props, base.TransformContext{Project: "p", Operation: tt.op})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %#v\nwant %#v", got, tt.want)
			}
		})
	}
}

func TestParentPolicyOf(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "a rule path yields its policy",
			in:   "projects/p/locations/europe-central2/gatewaySecurityPolicies/pol/rules/r1",
			want: "pol",
		},
		{
			name: "a flat resource path yields nothing",
			in:   "projects/p/locations/global/clientTlsPolicies/ctp",
			want: "",
		},
		{
			name: "a short name yields nothing rather than a wrong parent",
			in:   "r1",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parentPolicyOf(tt.in); got != tt.want {
				t.Errorf("parentPolicyOf(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestGatewaySecurityPolicyRuleResponseTransformer(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]interface{}
		want map[string]interface{}
	}{
		{
			// The empty applicationMatcher is invented by the API for a rule
			// that never sent one; tlsInspectionEnabled is a top-level bool, so
			// its schema hint reaches it and it stays.
			name: "the parent is recovered and the invented matcher dropped",
			in: map[string]interface{}{
				"name":                 "projects/p/locations/europe-central2/gatewaySecurityPolicies/pol/rules/r1",
				"enabled":              true,
				"priority":             float64(100),
				"description":          "probe",
				"sessionMatcher":       `host() == "example.com"`,
				"applicationMatcher":   "",
				"tlsInspectionEnabled": false,
				"basicProfile":         "ALLOW",
			},
			want: map[string]interface{}{
				"name":                  "r1",
				"gatewaySecurityPolicy": "pol",
				"enabled":               true,
				"priority":              float64(100),
				"description":           "probe",
				"sessionMatcher":        `host() == "example.com"`,
				"tlsInspectionEnabled":  false,
				"basicProfile":          "ALLOW",
			},
		},
		{
			name: "a real application matcher survives",
			in: map[string]interface{}{
				"name":               "projects/p/locations/europe-central2/gatewaySecurityPolicies/pol/rules/r1",
				"applicationMatcher": `request.path.contains("/api")`,
			},
			want: map[string]interface{}{
				"name":                  "r1",
				"gatewaySecurityPolicy": "pol",
				"applicationMatcher":    `request.path.contains("/api")`,
			},
		},
		{
			// A response that is not shaped like a rule must not gain a parent
			// guessed out of the path.
			name: "a name that is not a rule path leaves the parent unset",
			in: map[string]interface{}{
				"name": "r1",
			},
			want: map[string]interface{}{
				"name": "r1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gatewaySecurityPolicyRuleResponseTransformer.Transform(tt.in, base.TransformContext{})
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %#v\nwant %#v", got, tt.want)
			}
		})
	}
}

// The parent property a forma declares must be exactly what a read reports
// back. It is dropped from the request because it is a path component, so the
// only thing that can put it back is the response transformer - and if the two
// halves disagree the declared value and stored state never converge.
func TestGatewaySecurityPolicyRuleParentRoundTrips(t *testing.T) {
	const policy = "formae-test-ns-gsprp-1234"

	sent, err := gatewaySecurityPolicyRuleRequestTransformer.Transform(
		map[string]interface{}{"name": "r1", "gatewaySecurityPolicy": policy},
		base.TransformContext{Operation: resource.OperationCreate})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, present := sent["gatewaySecurityPolicy"]; present {
		t.Fatalf("the parent must not reach the request body: %#v", sent)
	}

	read := gatewaySecurityPolicyRuleResponseTransformer.Transform(map[string]interface{}{
		"name": "projects/p/locations/europe-central2/gatewaySecurityPolicies/" + policy + "/rules/r1",
	}, base.TransformContext{})
	if read["gatewaySecurityPolicy"] != policy {
		t.Errorf("read reported parent %v, want %q", read["gatewaySecurityPolicy"], policy)
	}
}
