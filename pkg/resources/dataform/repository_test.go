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

func TestRepositoryRequestTransformer(t *testing.T) {
	tests := []struct {
		name  string
		props map[string]interface{}
		op    resource.Operation
		want  map[string]interface{}
	}{
		{
			// base reads the id out of "name" for ?repositoryId=, so a create
			// must keep it.
			name: "create keeps the name and the KMS key",
			props: map[string]interface{}{
				"name":        "repo",
				"displayName": "d",
				"kmsKeyName":  "projects/p/locations/l/keyRings/r/cryptoKeys/k",
			},
			op: resource.OperationCreate,
			want: map[string]interface{}{
				"name":        "repo",
				"displayName": "d",
				"kmsKeyName":  "projects/p/locations/l/keyRings/r/cryptoKeys/k",
			},
		},
		{
			// The updateMask is built from the body, and the API refuses a mask
			// naming kms_key_name whatever the value.
			name: "update drops both, so neither enters the update mask",
			props: map[string]interface{}{
				"name":        "repo",
				"displayName": "d",
				"kmsKeyName":  "projects/p/locations/l/keyRings/r/cryptoKeys/k",
				"labels":      map[string]interface{}{"a": "b"},
			},
			op: resource.OperationUpdate,
			want: map[string]interface{}{
				"displayName": "d",
				"labels":      map[string]interface{}{"a": "b"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repositoryRequestTransformer.Transform(
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

func TestRepositoryResponseTransformer(t *testing.T) {
	// The exact shape a live create answered with, plus the gitRemoteSettings a
	// live PATCH answered with.
	in := map[string]interface{}{
		"name":        "projects/p/locations/europe-central2/repositories/repo",
		"displayName": "d",
		"labels":      map[string]interface{}{"environment": "test"},
		"workspaceCompilationOverrides": map[string]interface{}{
			"defaultDatabase": "p",
			"schemaSuffix":    "s",
		},
		"gitRemoteSettings": map[string]interface{}{
			"url":                    "https://github.com/example/example.git",
			"defaultBranch":          "main",
			"effectiveDefaultBranch": "main",
			"tokenStatus":            "TOKEN_STATUS_UNSPECIFIED",
		},
		"createTime":       "2026-09-03T15:05:28.374105287Z",
		"internalMetadata": `{"state":"ACTIVE"}`,
		"teamFolderName":   "",
		"containingFolder": "projects/p/locations/europe-central2/folders/f",
		"serviceAccount":   "sa@p.iam.gserviceaccount.com",
		"dataEncryptionState": map[string]interface{}{
			"kmsKeyVersionName": "projects/p/locations/l/keyRings/r/cryptoKeys/k/cryptoKeyVersions/1",
		},
	}
	want := map[string]interface{}{
		"name":        "repo",
		"displayName": "d",
		"labels":      map[string]interface{}{"environment": "test"},
		"workspaceCompilationOverrides": map[string]interface{}{
			"defaultDatabase": "p",
			"schemaSuffix":    "s",
		},
		"gitRemoteSettings": map[string]interface{}{
			"url":           "https://github.com/example/example.git",
			"defaultBranch": "main",
		},
		"serviceAccount": "sa@p.iam.gserviceaccount.com",
	}
	got := repositoryResponseTransformer.Transform(in, base.TransformContext{})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
}

// A repository declared without gitRemoteSettings must not gain one, and the
// transformer must not panic reaching into an absent nested object.
func TestRepositoryResponseTransformerWithoutGitRemote(t *testing.T) {
	got := repositoryResponseTransformer.Transform(map[string]interface{}{
		"name":             "projects/p/locations/europe-central2/repositories/repo",
		"internalMetadata": `{"state":"ACTIVE"}`,
		"teamFolderName":   "",
	}, base.TransformContext{})
	want := map[string]interface{}{"name": "repo"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
}
