// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package dataform

import (
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

func TestDataformPathBuilder(t *testing.T) {
	tests := []struct {
		name string
		ctx  base.PathContext
		want string
	}{
		{
			name: "a repository collection",
			ctx: base.PathContext{
				Project: "p", Location: "europe-central2", ResourceType: "repositories",
			},
			want: "/projects/p/locations/europe-central2/repositories",
		},
		{
			name: "a repository by name",
			ctx: base.PathContext{
				Project: "p", Location: "europe-central2",
				ResourceType: "repositories", ResourceName: "repo",
			},
			want: "/projects/p/locations/europe-central2/repositories/repo",
		},
		{
			name: "a nested workspace under its repository",
			ctx: base.PathContext{
				Project: "p", Location: "europe-central2",
				ParentType: "repositories", ParentResource: "repo",
				ResourceType: "workspaces", ResourceName: "ws",
			},
			want: "/projects/p/locations/europe-central2/repositories/repo/workspaces/ws",
		},
		{
			// The reason parentCollectionOf exists: discovery lists with no
			// parent, and without the wildcard the URL would address a
			// collection that does not exist.
			name: "a parentless list falls back to the wildcard repository",
			ctx: base.PathContext{
				Project: "p", Location: "europe-central2", ResourceType: "workflowConfigs",
			},
			want: "/projects/p/locations/europe-central2/repositories/-/workflowConfigs",
		},
		{
			name: "the region stands in when only it is set",
			ctx: base.PathContext{
				Project: "p", Region: "europe-central2", ResourceType: "repositories",
			},
			want: "/projects/p/locations/europe-central2/repositories",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dataformPathBuilder(tt.ctx); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestParseDataformNativeID(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    base.PathContext
		wantErr bool
	}{
		{
			name: "a repository path",
			in:   "projects/p/locations/europe-central2/repositories/repo",
			want: base.PathContext{
				Project: "p", Location: "europe-central2", Region: "europe-central2",
				ResourceType: "repositories", ResourceName: "repo",
			},
		},
		{
			// The default path parser overwrites ResourceType as it walks, so
			// the repository would be dropped and the read would 404.
			name: "a nested path keeps its repository",
			in:   "projects/p/locations/europe-central2/repositories/repo/releaseConfigs/rc",
			want: base.PathContext{
				Project: "p", Location: "europe-central2", Region: "europe-central2",
				ParentType: "repositories", ParentResource: "repo",
				ResourceType: "releaseConfigs", ResourceName: "rc",
			},
		},
		{
			name:    "a path that is not a Dataform resource is rejected",
			in:      "projects/p/locations/europe-central2/instances/i",
			wantErr: true,
		},
		{
			name:    "a short name is rejected rather than half-parsed",
			in:      "repo",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDataformNativeID(tt.in)
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

func TestExtractDataformNativeID(t *testing.T) {
	// A wildcard list has no ParentResource in its context, so building the id
	// from the context would leave the "-" in the path. The response's own name
	// is the only source that works.
	got := extractDataformNativeID(
		map[string]interface{}{
			"name": "projects/p/locations/europe-central2/repositories/repo/workspaces/ws",
		},
		base.PathContext{Project: "p", Location: "europe-central2", ResourceType: "workspaces"})
	const want = "projects/p/locations/europe-central2/repositories/repo/workspaces/ws"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}

	// A response with no name at all falls back to the context.
	got = extractDataformNativeID(
		map[string]interface{}{},
		base.PathContext{
			Project: "p", Location: "europe-central2",
			ParentType: "repositories", ParentResource: "repo",
			ResourceType: "workspaces", ResourceName: "ws",
		})
	if got != want {
		t.Errorf("fallback: got  %q\nwant %q", got, want)
	}
}

func TestParentRepositoryOf(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "a nested path yields its repository",
			in:   "projects/p/locations/europe-central2/repositories/repo/workspaces/ws",
			want: "repo",
		},
		{
			name: "a repository's own path yields nothing",
			in:   "projects/p/locations/europe-central2/repositories/repo",
			want: "",
		},
		{
			name: "a short name yields nothing rather than a wrong parent",
			in:   "ws",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parentRepositoryOf(tt.in); got != tt.want {
				t.Errorf("parentRepositoryOf(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
