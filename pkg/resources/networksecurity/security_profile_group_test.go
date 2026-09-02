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

func TestExpandSecurityProfileRef(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		project string
		want    string
	}{
		{
			name:    "short name is expanded to a full path",
			value:   "my-profile",
			project: "proj-1",
			want:    "projects/proj-1/locations/global/securityProfiles/my-profile",
		},
		{
			name:    "a value that is already a path is left alone",
			value:   "projects/other/locations/global/securityProfiles/shared",
			project: "proj-1",
			want:    "projects/other/locations/global/securityProfiles/shared",
		},
		{
			name:    "empty stays empty rather than becoming a path to nothing",
			value:   "",
			project: "proj-1",
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expandSecurityProfileRef(tt.value, tt.project); got != tt.want {
				t.Errorf("expandSecurityProfileRef() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShortenSecurityProfileRef(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "full path reduces to the short name",
			value: "projects/proj-1/locations/global/securityProfiles/my-profile",
			want:  "my-profile",
		},
		{
			name:  "an already-short name is unchanged",
			value: "my-profile",
			want:  "my-profile",
		},
		{
			name:  "empty stays empty",
			value: "",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shortenSecurityProfileRef(tt.value); got != tt.want {
				t.Errorf("shortenSecurityProfileRef() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Expanding on the way out and shortening on the way back must be exact
// inverses. If they are not, the declared value and the stored state disagree
// forever and every re-apply plans a replacement of an unchanged resource.
func TestSecurityProfileRefRoundTrips(t *testing.T) {
	for _, short := range []string{"my-profile", "a", "profile-with-dashes-123"} {
		full := expandSecurityProfileRef(short, "proj-1")
		if got := shortenSecurityProfileRef(full); got != short {
			t.Errorf("round trip of %q via %q gave %q", short, full, got)
		}
	}
}

func TestSecurityProfileGroupRequestTransformer(t *testing.T) {
	tests := []struct {
		name  string
		props map[string]interface{}
		op    resource.Operation
		want  map[string]interface{}
	}{
		{
			name: "create expands every profile reference and keeps the name",
			props: map[string]interface{}{
				"name":                    "my-group",
				"threatPreventionProfile": "tp",
				"urlFilteringProfile":     "uf",
				"customMirroringProfile":  "cm",
				"customInterceptProfile":  "ci",
				"description":             "d",
			},
			op: resource.OperationCreate,
			want: map[string]interface{}{
				"name":                    "my-group",
				"threatPreventionProfile": "projects/p/locations/global/securityProfiles/tp",
				"urlFilteringProfile":     "projects/p/locations/global/securityProfiles/uf",
				"customMirroringProfile":  "projects/p/locations/global/securityProfiles/cm",
				"customInterceptProfile":  "projects/p/locations/global/securityProfiles/ci",
				"description":             "d",
			},
		},
		{
			name: "update drops the name so it cannot enter the update mask",
			props: map[string]interface{}{
				"name":                    "my-group",
				"threatPreventionProfile": "tp",
				"description":             "d",
			},
			op: resource.OperationUpdate,
			want: map[string]interface{}{
				"threatPreventionProfile": "projects/p/locations/global/securityProfiles/tp",
				"description":             "d",
			},
		},
		{
			name: "server-owned fields never reach the wire",
			props: map[string]interface{}{
				"name":       "my-group",
				"etag":       "abc",
				"dataPathId": "3170",
			},
			op: resource.OperationCreate,
			want: map[string]interface{}{
				"name": "my-group",
			},
		},
		{
			name: "an unset profile reference is not invented",
			props: map[string]interface{}{
				"name": "my-group",
			},
			op: resource.OperationCreate,
			want: map[string]interface{}{
				"name": "my-group",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := securityProfileGroupRequestTransformer(
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

func TestSecurityProfileGroupResponseTransformer(t *testing.T) {
	got := securityProfileGroupResponseTransformer(map[string]interface{}{
		"name":                    "projects/p/locations/global/securityProfileGroups/my-group",
		"threatPreventionProfile": "projects/p/locations/global/securityProfiles/tp",
		"urlFilteringProfile":     "projects/p/locations/global/securityProfiles/uf",
		"customMirroringProfile":  "projects/p/locations/global/securityProfiles/cm",
		"customInterceptProfile":  "projects/p/locations/global/securityProfiles/ci",
		"dataPathId":              "3170",
	}, base.TransformContext{})

	want := map[string]interface{}{
		"name":                    "my-group",
		"threatPreventionProfile": "tp",
		"urlFilteringProfile":     "uf",
		"customMirroringProfile":  "cm",
		"customInterceptProfile":  "ci",
		"dataPathId":              "3170",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
}

// A response missing the optional references must not gain them as empty
// strings, which would read as a declared-but-blank reference.
func TestSecurityProfileGroupResponseTransformerLeavesAbsentRefsAbsent(t *testing.T) {
	got := securityProfileGroupResponseTransformer(map[string]interface{}{
		"name": "projects/p/locations/global/securityProfileGroups/my-group",
	}, base.TransformContext{})

	if _, present := got["threatPreventionProfile"]; present {
		t.Errorf("absent reference was materialised: %#v", got)
	}
}
