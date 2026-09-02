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

func TestSecurityProfileRequestTransformer(t *testing.T) {
	tests := []struct {
		name  string
		props map[string]interface{}
		op    resource.Operation
		want  map[string]interface{}
	}{
		{
			name: "create keeps name and type but never the etag",
			props: map[string]interface{}{
				"name":        "my-profile",
				"type":        "THREAT_PREVENTION",
				"description": "d",
				"etag":        "abc",
			},
			op: resource.OperationCreate,
			want: map[string]interface{}{
				"name":        "my-profile",
				"type":        "THREAT_PREVENTION",
				"description": "d",
			},
		},
		{
			name: "update drops name and type so neither enters the update mask",
			props: map[string]interface{}{
				"name":        "my-profile",
				"type":        "THREAT_PREVENTION",
				"description": "d",
				"labels":      map[string]interface{}{"a": "1"},
				"etag":        "abc",
			},
			op: resource.OperationUpdate,
			want: map[string]interface{}{
				"description": "d",
				"labels":      map[string]interface{}{"a": "1"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := securityProfileRequestTransformer(
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

// threatOverrides[].type is output-only and nested, so no schema hint can reach
// it. Left in the response it is an undeclared property and Verify rejects the
// read.
func TestSecurityProfileResponseTransformerStripsNestedOutputOnlyType(t *testing.T) {
	got := securityProfileResponseTransformer(map[string]interface{}{
		"name": "projects/p/locations/global/securityProfiles/my-profile",
		"threatPreventionProfile": map[string]interface{}{
			"threatOverrides": []interface{}{
				map[string]interface{}{
					"threatId": "12345",
					"action":   "DENY",
					"type":     "VULNERABILITY",
				},
				map[string]interface{}{
					"threatId": "67890",
					"action":   "ALERT",
					"type":     "SPYWARE",
				},
			},
			"severityOverrides": []interface{}{
				map[string]interface{}{"severity": "HIGH", "action": "DENY"},
			},
		},
	}, base.TransformContext{})

	want := map[string]interface{}{
		"name": "my-profile",
		"threatPreventionProfile": map[string]interface{}{
			"threatOverrides": []interface{}{
				map[string]interface{}{"threatId": "12345", "action": "DENY"},
				map[string]interface{}{"threatId": "67890", "action": "ALERT"},
			},
			"severityOverrides": []interface{}{
				map[string]interface{}{"severity": "HIGH", "action": "DENY"},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
}

// The severity override's own "action" is user-declarable; only the threat
// override's "type" is output-only. Stripping by field name alone would eat the
// wrong thing.
func TestSecurityProfileResponseTransformerHandlesShapesItDoesNotExpect(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]interface{}
		want map[string]interface{}
	}{
		{
			name: "no threat prevention block at all",
			in:   map[string]interface{}{"name": "projects/p/locations/global/securityProfiles/x"},
			want: map[string]interface{}{"name": "x"},
		},
		{
			name: "empty threat prevention block, as GCP reports for a bare profile",
			in: map[string]interface{}{
				"name":                    "projects/p/locations/global/securityProfiles/x",
				"threatPreventionProfile": map[string]interface{}{},
			},
			want: map[string]interface{}{
				"name":                    "x",
				"threatPreventionProfile": map[string]interface{}{},
			},
		},
		{
			name: "a url filtering profile is untouched",
			in: map[string]interface{}{
				"name": "projects/p/locations/global/securityProfiles/x",
				"urlFilteringProfile": map[string]interface{}{
					"urlFilters": []interface{}{
						map[string]interface{}{"urls": []interface{}{"a.test"}, "filteringAction": "DENY", "priority": float64(1)},
					},
				},
			},
			want: map[string]interface{}{
				"name": "x",
				"urlFilteringProfile": map[string]interface{}{
					"urlFilters": []interface{}{
						map[string]interface{}{"urls": []interface{}{"a.test"}, "filteringAction": "DENY", "priority": float64(1)},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := securityProfileResponseTransformer(tt.in, base.TransformContext{})
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %#v\nwant %#v", got, tt.want)
			}
		})
	}
}

// urlLists is the one regional collection here; everything else must resolve to
// global whatever region the target carries.
func TestLocationOf(t *testing.T) {
	tests := []struct {
		resourceType string
		location     string
		want         string
	}{
		{"addressGroups", "europe-central2", "global"},
		{"securityProfiles", "europe-central2", "global"},
		{"securityProfileGroups", "europe-central2", "global"},
		{"urlLists", "europe-central2", "europe-central2"},
		{"urlLists", "", "global"},
	}
	for _, tt := range tests {
		t.Run(tt.resourceType+"/"+tt.location, func(t *testing.T) {
			got := locationOf(base.PathContext{ResourceType: tt.resourceType, Location: tt.location})
			if got != tt.want {
				t.Errorf("locationOf(%s, %q) = %q, want %q", tt.resourceType, tt.location, got, tt.want)
			}
		})
	}
}
