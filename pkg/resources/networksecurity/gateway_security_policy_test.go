// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package networksecurity

import (
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// A gateway security policy created without a TLS inspection policy is reported
// with tlsInspectionPolicy = "". Left in place, state carries a value against a
// declaration that omits the field, and every sync reads it as drift.
func TestGatewaySecurityPolicyResponseTransformer(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]interface{}
		want map[string]interface{}
	}{
		{
			name: "the invented empty tlsInspectionPolicy is dropped",
			in: map[string]interface{}{
				"name":                "projects/p/locations/europe-central2/gatewaySecurityPolicies/pol",
				"description":         "probe",
				"tlsInspectionPolicy": "",
			},
			want: map[string]interface{}{
				"name":        "pol",
				"description": "probe",
			},
		},
		{
			name: "a policy that really has one keeps it",
			in: map[string]interface{}{
				"name":                "projects/p/locations/europe-central2/gatewaySecurityPolicies/pol",
				"tlsInspectionPolicy": "projects/p/locations/europe-central2/tlsInspectionPolicies/tip",
			},
			want: map[string]interface{}{
				"name":                "pol",
				"tlsInspectionPolicy": "projects/p/locations/europe-central2/tlsInspectionPolicies/tip",
			},
		},
		{
			name: "an absent field is not materialised",
			in: map[string]interface{}{
				"name": "projects/p/locations/europe-central2/gatewaySecurityPolicies/pol",
			},
			want: map[string]interface{}{
				"name": "pol",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gatewaySecurityPolicyResponseTransformer.Transform(tt.in, base.TransformContext{})
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %#v\nwant %#v", got, tt.want)
			}
		})
	}
}

// Only the empty string goes. A false, a zero or a nested empty object is a
// value the API meant to report.
func TestDropEmptyStrings(t *testing.T) {
	in := map[string]interface{}{
		"gone":       "",
		"kept":       "x",
		"notAString": false,
		"alsoKept":   0,
	}
	want := map[string]interface{}{
		"kept":       "x",
		"notAString": false,
		"alsoKept":   0,
	}
	got := dropEmptyStrings("gone", "notAString", "missing").Transform(in, base.TransformContext{})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
}
