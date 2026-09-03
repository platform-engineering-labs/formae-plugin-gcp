// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package parametermanager

import (
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// Every URL this plugin builds addresses locations/global, whatever the target
// says. A region here is the 403 that reads like a missing IAM grant: a
// regional parameter is only reachable on that region's own rep. host, and
// base cannot vary the host. See globalLocation in api.go.
func TestPathBuilderIsAlwaysGlobal(t *testing.T) {
	tests := []struct {
		name string
		ctx  base.PathContext
		want string
	}{
		{
			name: "parameter collection",
			ctx:  base.PathContext{Project: "p", ResourceType: "parameters"},
			want: "/projects/p/locations/global/parameters",
		},
		{
			name: "a target's region never reaches the URL",
			ctx: base.PathContext{
				Project:      "p",
				Region:       "europe-central2",
				Location:     "europe-central2",
				ResourceType: "parameters",
				ResourceName: "param",
			},
			want: "/projects/p/locations/global/parameters/param",
		},
		{
			name: "a version under its parameter",
			ctx: base.PathContext{
				Project:        "p",
				ResourceType:   "versions",
				ResourceName:   "v1",
				ParentType:     "parameters",
				ParentResource: "param",
			},
			want: "/projects/p/locations/global/parameters/param/versions/v1",
		},
		{
			name: "a parentless version list takes the wildcard parent",
			ctx:  base.PathContext{Project: "p", ResourceType: "versions", IsList: true},
			want: "/projects/p/locations/global/parameters/-/versions",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parameterManagerPathBuilder(tt.ctx); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

// A version's native ID must parse back into a context that still knows its
// parent; base's default path parser would overwrite ResourceType and address
// ".../locations/global/versions/{version}", which 404s.
func TestParseNativeID(t *testing.T) {
	tests := []struct {
		name     string
		nativeID string
		want     base.PathContext
		wantErr  bool
	}{
		{
			name:     "parameter",
			nativeID: "projects/p/locations/global/parameters/param",
			want: base.PathContext{
				Project: "p", Location: "global",
				ResourceType: "parameters", ResourceName: "param",
			},
		},
		{
			name:     "version keeps its parent",
			nativeID: "projects/p/locations/global/parameters/param/versions/v1",
			want: base.PathContext{
				Project: "p", Location: "global",
				ResourceType: "versions", ResourceName: "v1",
				ParentType: "parameters", ParentResource: "param",
			},
		},
		{name: "too short", nativeID: "projects/p/locations/global", wantErr: true},
		{name: "not a project path", nativeID: "folders/f/locations/global/parameters/x", wantErr: true},
		{
			name:     "unexpected depth",
			nativeID: "projects/p/locations/global/parameters/param/versions/v1/extra/x",
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseParameterManagerNativeID(tt.nativeID)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %#v\nwant %#v", got, tt.want)
			}
		})
	}
}

// The native ID comes out of the response's own name where there is one - this
// API always answers a mutation with the resource - and out of the context
// otherwise.
func TestExtractNativeID(t *testing.T) {
	tests := []struct {
		name     string
		response map[string]interface{}
		ctx      base.PathContext
		want     string
	}{
		{
			name:     "from the response name",
			response: map[string]interface{}{"name": "projects/p/locations/global/parameters/param"},
			want:     "projects/p/locations/global/parameters/param",
		},
		{
			name:     "a nested version's name already carries its parent",
			response: map[string]interface{}{"name": versionPath},
			want:     versionPath,
		},
		{
			name:     "from the context when the response has no name",
			response: map[string]interface{}{},
			ctx: base.PathContext{
				Project: "p", ResourceType: "versions", ResourceName: "v1",
				ParentType: "parameters", ParentResource: "param",
			},
			want: versionPath,
		},
		{
			name:     "nothing to build from",
			response: map[string]interface{}{},
			ctx:      base.PathContext{Project: "p", ResourceType: "parameters"},
			want:     "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractParameterManagerNativeID(tt.response, tt.ctx); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

// This API has no operations collection, so nothing is ever an operation name.
func TestExtractOperationNameIsAlwaysEmpty(t *testing.T) {
	if got := extractOperationName(map[string]interface{}{"name": versionPath}); got != "" {
		t.Errorf("got %q, want \"\"", got)
	}
}
