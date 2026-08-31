// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package storage

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// The names of every storage type that existed before managed folders are a
// single segment. These pin that the parser and the path builder still treat
// them exactly as they did - this is the half of the change that must not move.
func TestExistingStorageNamesAreUnchanged(t *testing.T) {
	cases := map[string]struct {
		nativeID string
		wantPath string
	}{
		"bucket acl entity": {
			"b/my-bucket/acl/user-someone@example.com",
			"/b/my-bucket/acl/user-someone@example.com",
		},
		"default object acl": {
			"b/my-bucket/defaultObjectAcl/allUsers",
			"/b/my-bucket/defaultObjectAcl/allUsers",
		},
		"anywhere cache": {
			"b/my-bucket/anywhereCaches/cache-1",
			"/b/my-bucket/anywhereCaches/cache-1",
		},
	}
	for name, tc := range cases {
		ctx, err := parseStorageNativeID(tc.nativeID)
		if err != nil {
			t.Errorf("%s: parse: %v", name, err)
			continue
		}
		if got := storagePathBuilder(ctx); got != tc.wantPath {
			t.Errorf("%s: path = %q, want %q", name, got, tc.wantPath)
		}
	}
}

// A managed folder is named with a trailing slash that is part of its identity:
// "reports/" is not "reports". Splitting on "/" and taking one segment dropped
// it, and the rebuilt URL then addressed a folder that does not exist.
func TestSlashTerminatedNamesSurviveAndAreEscaped(t *testing.T) {
	ctx, err := parseStorageNativeID("b/my-bucket/managedFolders/reports/")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ctx.ParentResource != "my-bucket" || ctx.ResourceType != "managedFolders" {
		t.Errorf("ctx = %+v", ctx)
	}
	if ctx.ResourceName != "reports/" {
		t.Errorf("name = %q, want %q", ctx.ResourceName, "reports/")
	}
	// The slash has to be escaped or it reads as another path segment.
	want := "/b/my-bucket/managedFolders/reports%2F"
	if got := storagePathBuilder(ctx); got != want {
		t.Errorf("path = %q, want %q", got, want)
	}

	// Nested folders keep every segment.
	nested, err := parseStorageNativeID("b/my-bucket/folders/a/b/")
	if err != nil {
		t.Fatalf("parse nested: %v", err)
	}
	if nested.ResourceName != "a/b/" {
		t.Errorf("nested name = %q", nested.ResourceName)
	}
}

// A bucket is still addressed by a bare name, and a project-scoped resource
// still by its project path.
func TestBucketAndProjectScopedShapesStillParse(t *testing.T) {
	b, err := parseStorageNativeID("my-bucket")
	if err != nil || b.ResourceType != "b" || b.ResourceName != "my-bucket" {
		t.Errorf("bucket ctx = %+v err=%v", b, err)
	}
	p, err := parseStorageNativeID("projects/proj/hmacKeys/GOOG1EXAMPLE")
	if err != nil || p.ResourceType != "hmacKeys" || p.ResourceName != "GOOG1EXAMPLE" || p.ParentResource != "" {
		t.Errorf("project ctx = %+v err=%v", p, err)
	}
	if got := storagePathBuilder(base.PathContext{Project: "proj", ResourceType: "hmacKeys", ResourceName: "GOOG1EXAMPLE"}); got != "/projects/proj/hmacKeys/GOOG1EXAMPLE" {
		t.Errorf("project path = %q", got)
	}
}
