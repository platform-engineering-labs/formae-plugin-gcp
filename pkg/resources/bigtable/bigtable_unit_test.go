// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package bigtable

import "testing"

// The parser used to switch on each collection by name, so a collection nobody
// had listed parsed to an empty resource type and read nothing - silently.
// Every instance-scoped collection now falls through one default branch.
func TestNativeIDParsesAnyInstanceScopedCollection(t *testing.T) {
	cases := map[string]struct {
		nativeID     string
		resourceType string
		resourceName string
		parent       string
	}{
		"cluster":          {"projects/p/instances/i/clusters/c", "clusters", "c", "i"},
		"table":            {"projects/p/instances/i/tables/t", "tables", "t", "i"},
		"materializedView": {"projects/p/instances/i/materializedViews/mv", "materializedViews", "mv", "i"},
		"appProfile":       {"projects/p/instances/i/appProfiles/ap", "appProfiles", "ap", "i"},
		"logicalView":      {"projects/p/instances/i/logicalViews/lv", "logicalViews", "lv", "i"},
	}
	for name, tc := range cases {
		ctx, err := parseBigtableNativeID(tc.nativeID)
		if err != nil {
			t.Errorf("%s: parse: %v", name, err)
			continue
		}
		if ctx.ResourceType != tc.resourceType || ctx.ResourceName != tc.resourceName {
			t.Errorf("%s: resource = %s/%s, want %s/%s",
				name, ctx.ResourceType, ctx.ResourceName, tc.resourceType, tc.resourceName)
		}
		if ctx.ParentResource != tc.parent {
			t.Errorf("%s: parent = %q, want %q", name, ctx.ParentResource, tc.parent)
		}
		if ctx.Project != "p" {
			t.Errorf("%s: project = %q", name, ctx.Project)
		}
	}
}

// A standalone instance is not a parent of anything.
func TestNativeIDParsesAStandaloneInstance(t *testing.T) {
	ctx, err := parseBigtableNativeID("projects/p/instances/i")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ctx.ResourceType != "instances" || ctx.ResourceName != "i" || ctx.ParentResource != "" {
		t.Errorf("ctx = %+v", ctx)
	}
}

// Backups are the one genuinely three-deep shape: the cluster goes to
// CustomSegments so the path builder can put it back.
func TestNativeIDKeepsTheClusterForABackup(t *testing.T) {
	ctx, err := parseBigtableNativeID("projects/p/instances/i/clusters/c/backups/b")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ctx.ResourceType != "backups" || ctx.ResourceName != "b" || ctx.ParentResource != "i" {
		t.Errorf("ctx = %+v", ctx)
	}
	if len(ctx.CustomSegments) != 1 || ctx.CustomSegments[0] != "c" {
		t.Errorf("cluster = %v", ctx.CustomSegments)
	}
}
