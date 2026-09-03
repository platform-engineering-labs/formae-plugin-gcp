// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package dataform

import (
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// The repository property a forma declares must be exactly what a read reports
// back. It is dropped from the request because the API rejects it as an unknown
// body field, so the only thing that can put it back is the response
// transformer - and if the two halves disagree the declared value and stored
// state never converge, which on a create-only field means every re-apply plans
// a replacement.
func TestRepositoryPropertyRoundTrips(t *testing.T) {
	const repo = "formae-test-df-wsr-1234"

	cases := []struct {
		name     string
		request  base.RequestTransformer
		response base.ResponseTransformer
		path     string
	}{
		{
			name:     "workspace",
			request:  workspaceRequestTransformer,
			response: workspaceResponseTransformer,
			path:     "projects/p/locations/europe-central2/repositories/" + repo + "/workspaces/ws",
		},
		{
			name:     "releaseConfig",
			request:  releaseConfigRequestTransformer,
			response: releaseConfigResponseTransformer,
			path:     "projects/p/locations/europe-central2/repositories/" + repo + "/releaseConfigs/rc",
		},
		{
			name:     "workflowConfig",
			request:  workflowConfigRequestTransformer,
			response: workflowConfigResponseTransformer,
			path:     "projects/p/locations/europe-central2/repositories/" + repo + "/workflowConfigs/wc",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sent, err := tc.request.Transform(
				map[string]interface{}{"name": "x", "repository": repo},
				base.TransformContext{
					Project: "p", Location: "europe-central2",
					ParentResource: repo, Operation: resource.OperationCreate,
				})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if _, present := sent["repository"]; present {
				t.Fatalf("the repository must not reach the request body: %#v", sent)
			}

			read := tc.response.Transform(
				map[string]interface{}{"name": tc.path}, base.TransformContext{})
			if read["repository"] != repo {
				t.Errorf("read reported repository %v, want %q", read["repository"], repo)
			}
		})
	}
}

func TestWorkspaceResponseTransformer(t *testing.T) {
	// The exact shape a live create answered with.
	in := map[string]interface{}{
		"name":             "projects/p/locations/europe-central2/repositories/repo/workspaces/ws",
		"createTime":       "2026-09-03T15:05:48.517285790Z",
		"internalMetadata": `{"unique_ccfe_id":"13fbe6fb"}`,
		"disableMoves":     false,
		"privateResourceMetadata": map[string]interface{}{
			"userScoped": true,
		},
		"dataEncryptionState": map[string]interface{}{
			"kmsKeyVersionName": "projects/p/locations/l/keyRings/r/cryptoKeys/k/cryptoKeyVersions/1",
		},
	}
	// disableMoves survives: it is a top-level bool a forma may set, and its
	// false-when-unset is covered by hasProviderDefault in the schema.
	want := map[string]interface{}{
		"name":         "ws",
		"repository":   "repo",
		"disableMoves": false,
	}
	got := workspaceResponseTransformer.Transform(in, base.TransformContext{})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
}

func TestReleaseConfigRequestTransformer(t *testing.T) {
	tests := []struct {
		name  string
		props map[string]interface{}
		op    resource.Operation
		want  map[string]interface{}
	}{
		{
			name: "create keeps the name and the compilation config",
			props: map[string]interface{}{
				"name":                  "rc",
				"repository":            "repo",
				"gitCommitish":          "main",
				"codeCompilationConfig": map[string]interface{}{"defaultSchema": "s"},
			},
			op: resource.OperationCreate,
			want: map[string]interface{}{
				"name":                  "rc",
				"gitCommitish":          "main",
				"codeCompilationConfig": map[string]interface{}{"defaultSchema": "s"},
			},
		},
		{
			// The API refuses a mask naming code_compilation_config whatever
			// the value, so it must leave the body a reconcile PATCH sends.
			name: "update drops the name and the immutable compilation config",
			props: map[string]interface{}{
				"name":                  "rc",
				"repository":            "repo",
				"gitCommitish":          "release",
				"timeZone":              "UTC",
				"disabled":              true,
				"codeCompilationConfig": map[string]interface{}{"defaultSchema": "s"},
			},
			op: resource.OperationUpdate,
			want: map[string]interface{}{
				"gitCommitish": "release",
				"timeZone":     "UTC",
				"disabled":     true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := releaseConfigRequestTransformer.Transform(
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

func TestReleaseConfigResponseTransformer(t *testing.T) {
	in := map[string]interface{}{
		"name":                     "projects/p/locations/europe-central2/repositories/repo/releaseConfigs/rc",
		"gitCommitish":             "main",
		"timeZone":                 "UTC",
		"disabled":                 true,
		"internalMetadata":         `{"unique_ccfe_id":"6248b122"}`,
		"releaseCompilationResult": "projects/p/locations/europe-central2/repositories/repo/compilationResults/1",
		"recentScheduledReleaseRecords": []interface{}{
			map[string]interface{}{"releaseTime": "2026-09-03T15:00:00Z"},
		},
		"codeCompilationConfig": map[string]interface{}{"defaultSchema": "s"},
	}
	want := map[string]interface{}{
		"name":                  "rc",
		"repository":            "repo",
		"gitCommitish":          "main",
		"timeZone":              "UTC",
		"disabled":              true,
		"codeCompilationConfig": map[string]interface{}{"defaultSchema": "s"},
	}
	got := releaseConfigResponseTransformer.Transform(in, base.TransformContext{})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
}
