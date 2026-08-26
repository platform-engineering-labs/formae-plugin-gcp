// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package alloydb

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

const instancePath = "projects/dev-1/locations/europe-central2/clusters/c1/instances/i1"

// The default pairwise path parser would keep only the last type/name pair and
// lose the owning cluster, which the instance's URL needs.
func TestParseInstanceNativeIDKeepsCluster(t *testing.T) {
	ctx, err := AlloyDBInstanceNativeID.Parser(instancePath)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Project != "dev-1" || ctx.Location != "europe-central2" {
		t.Errorf("project/location: %q %q", ctx.Project, ctx.Location)
	}
	if ctx.ParentType != "clusters" || ctx.ParentResource != "c1" {
		t.Errorf("parent not parsed: %q %q", ctx.ParentType, ctx.ParentResource)
	}
	if ctx.ResourceType != "instances" || ctx.ResourceName != "i1" {
		t.Errorf("resource: %q %q", ctx.ResourceType, ctx.ResourceName)
	}
}

func TestParseInstanceNativeIDRejectsOtherShapes(t *testing.T) {
	for _, bad := range []string{
		"projects/p/locations/l/clusters/c1",            // the cluster itself
		"projects/p/locations/l/instances/i1",           // missing the cluster
		"projects/p/locations/l/clusters/c1/backups/b1", // wrong leaf type
		"",
	} {
		if _, err := AlloyDBInstanceNativeID.Parser(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

// Create is async: the response is an Operation, not the instance, so the native
// ID has to be composed from context.
func TestExtractInstanceNativeIDFromContext(t *testing.T) {
	got := extractInstanceNativeID(
		map[string]interface{}{"name": "projects/dev-1/locations/europe-central2/operations/op-123"},
		base.PathContext{
			Project: "dev-1", Location: "europe-central2",
			ParentResource: "c1", ResourceName: "i1",
		})
	if got != instancePath {
		t.Errorf("composed native ID: %q", got)
	}
}

// Without ResourceName (e.g. a discovery-side read) it falls back to the
// operation's metadata target.
func TestExtractInstanceNativeIDFromOperationMetadata(t *testing.T) {
	got := extractInstanceNativeID(map[string]interface{}{
		"name": "projects/dev-1/locations/europe-central2/operations/op-123",
		"metadata": map[string]interface{}{
			"target": "https://alloydb.googleapis.com/v1/" + instancePath,
		},
	}, base.PathContext{})
	if got != instancePath {
		t.Errorf("metadata fallback: %q", got)
	}
}

// A direct GET returns the instance itself, whose name is already the full path.
func TestExtractInstanceNativeIDFromDirectRead(t *testing.T) {
	got := extractInstanceNativeID(map[string]interface{}{"name": instancePath}, base.PathContext{})
	if got != instancePath {
		t.Errorf("direct read: %q", got)
	}
}

// The API never returns a "cluster" field - it is a path component - so it must
// be lifted out of the name or the stored state will not match the forma.
func TestInstanceClusterLiftedFromName(t *testing.T) {
	out := instanceClusterFromName.Transform(
		map[string]interface{}{"name": instancePath}, base.TransformContext{})
	if out["cluster"] != "c1" {
		t.Errorf("cluster not lifted: %#v", out["cluster"])
	}
}

// An unparseable name must not gain a bogus cluster field.
func TestInstanceClusterAbsentWhenNoClusterSegment(t *testing.T) {
	out := instanceClusterFromName.Transform(
		map[string]interface{}{"name": "projects/p/locations/l/instances/i1"}, base.TransformContext{})
	if _, ok := out["cluster"]; ok {
		t.Errorf("must not invent a cluster: %#v", out)
	}
}

// The parser factory is shared by instances and users; each must reject the
// other's ids rather than mis-parse them.
func TestClusterScopedNativeIDIsTypeSpecific(t *testing.T) {
	userPath := "projects/dev-1/locations/europe-central2/clusters/c1/users/u1"
	if _, err := AlloyDBUserNativeID.Parser(userPath); err != nil {
		t.Errorf("user parser should accept a user path: %v", err)
	}
	if _, err := AlloyDBInstanceNativeID.Parser(userPath); err == nil {
		t.Errorf("instance parser must reject a user path")
	}
	if _, err := AlloyDBUserNativeID.Parser(instancePath); err == nil {
		t.Errorf("user parser must reject an instance path")
	}
	ctx, err := AlloyDBUserNativeID.Parser(userPath)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.ParentResource != "c1" || ctx.ResourceType != "users" || ctx.ResourceName != "u1" {
		t.Errorf("user parse: %#v", ctx)
	}
}

// A discovery List names no cluster, so the builder has to supply both halves
// of the "clusters/-" segment itself - PathContext.ParentType is empty too.
func TestInstanceListPathUsesClusterWildcard(t *testing.T) {
	got := alloyDBPathBuilder(base.PathContext{
		Project:      "p",
		Location:     "europe-central2",
		ResourceType: "instances",
		IsList:       true,
	})
	want := "/projects/p/locations/europe-central2/clusters/-/instances"
	if got != want {
		t.Errorf("list path = %q, want %q", got, want)
	}
}

// A named cluster still wins over the wildcard.
func TestInstanceListPathKeepsNamedCluster(t *testing.T) {
	got := alloyDBPathBuilder(base.PathContext{
		Project:        "p",
		Location:       "europe-central2",
		ParentType:     "clusters",
		ParentResource: "c1",
		ResourceType:   "instances",
		IsList:         true,
	})
	want := "/projects/p/locations/europe-central2/clusters/c1/instances"
	if got != want {
		t.Errorf("list path = %q, want %q", got, want)
	}
}
