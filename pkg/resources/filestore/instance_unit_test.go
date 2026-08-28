// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package filestore

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestPathBuilder(t *testing.T) {
	ctx := base.PathContext{Project: "p", Location: "us-central1", ResourceType: "instances"}
	if got := filestorePathBuilder(ctx); got != "/projects/p/locations/us-central1/instances" {
		t.Errorf("collection path = %q", got)
	}
	ctx.ResourceName = "fs1"
	if got := filestorePathBuilder(ctx); got != "/projects/p/locations/us-central1/instances/fs1" {
		t.Errorf("resource path = %q", got)
	}
}

func TestOperationName(t *testing.T) {
	// A create/delete response is an Operation.
	if got := extractOperationName(map[string]interface{}{
		"name": "projects/p/locations/us-central1/operations/op9",
	}); got != "projects/p/locations/us-central1/operations/op9" {
		t.Errorf("op name = %q", got)
	}
	// A direct resource response is NOT an operation.
	if got := extractOperationName(map[string]interface{}{
		"name": "projects/p/locations/us-central1/instances/fs1",
	}); got != "" {
		t.Errorf("resource name should not be treated as op: %q", got)
	}
}

func TestNativeIDFromOperationContext(t *testing.T) {
	// Async create: response is an Operation; native ID built from context.
	ctx := base.PathContext{Project: "p", Location: "us-central1", ResourceType: "instances", ResourceName: "fs1"}
	got := extractFilestoreNativeID(
		map[string]interface{}{"name": "projects/p/locations/us-central1/operations/op9", "done": false}, ctx)
	if got != "projects/p/locations/us-central1/instances/fs1" {
		t.Errorf("native id = %q", got)
	}
}

func TestNativeIDFromMetadataTarget(t *testing.T) {
	// No context name: fall back to the operation's metadata.target.
	ctx := base.PathContext{}
	got := extractFilestoreNativeID(map[string]interface{}{
		"name": "projects/p/locations/us-central1/operations/op9",
		"metadata": map[string]interface{}{
			"target": "projects/p/locations/us-central1/instances/fs1",
		},
	}, ctx)
	if got != "projects/p/locations/us-central1/instances/fs1" {
		t.Errorf("native id from metadata = %q", got)
	}
}

func TestOperationStatusChecker(t *testing.T) {
	if done, err := checkOperationStatus(map[string]interface{}{"done": false}); done || err != nil {
		t.Errorf("in-progress: got (%v,%v)", done, err)
	}
	if done, err := checkOperationStatus(map[string]interface{}{"done": true}); !done || err != nil {
		t.Errorf("success: got (%v,%v)", done, err)
	}
	done, err := checkOperationStatus(map[string]interface{}{
		"done": true, "error": map[string]interface{}{"message": "boom"}})
	if !done || err == nil || err.Error() != "boom" {
		t.Errorf("failure: got (%v,%v)", done, err)
	}
}

func TestInstanceRegistered(t *testing.T) {
	for _, op := range []resource.Operation{
		resource.OperationCreate, resource.OperationRead,
		resource.OperationDelete, resource.OperationList,
	} {
		if !registry.HasProvisioner(InstanceResourceType, op) {
			t.Errorf("%s not registered for %v", InstanceResourceType, op)
		}
	}
}

// A snapshot is addressed inside its instance; instances and backups are not.
func TestFilestorePathBuilderNesting(t *testing.T) {
	got := filestorePathBuilder(base.PathContext{
		Project: "p", Location: "z",
		ParentType: "instances", ParentResource: "fs1",
		ResourceType: "snapshots", ResourceName: "s1",
	})
	if want := "/projects/p/locations/z/instances/fs1/snapshots/s1"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	got = filestorePathBuilder(base.PathContext{
		Project: "p", Location: "r", ResourceType: "backups", ResourceName: "b1",
	})
	if want := "/projects/p/locations/r/backups/b1"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFilestoreNativeIDParser(t *testing.T) {
	ctx, err := parseFilestoreNativeID("projects/p/locations/z/instances/fs1/snapshots/s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.ParentType != "instances" || ctx.ParentResource != "fs1" ||
		ctx.ResourceType != "snapshots" || ctx.ResourceName != "s1" {
		t.Errorf("nested parse wrong: %+v", ctx)
	}

	ctx, err = parseFilestoreNativeID("projects/p/locations/r/backups/b1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.ResourceType != "backups" || ctx.ResourceName != "b1" || ctx.Location != "r" {
		t.Errorf("top-level parse wrong: %+v", ctx)
	}

	if _, err := parseFilestoreNativeID("projects/p/locations/z/instances"); err == nil {
		t.Error("a collection path is not a resource and must be rejected")
	}
}

// A forma passes a resolvable that resolves to a bare instance name; the API
// wants a full path. Expand on write, shorten on read - otherwise every
// comparison step reports drift on a backup that is in fact correct.
func TestBackupSourceInstanceRoundTrip(t *testing.T) {
	body, err := backupRequest(map[string]interface{}{
		"name":           "b1",
		"location":       "z",
		"sourceInstance": "fs1",
	}, base.TransformContext{Project: "p", Location: "z"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := body["sourceInstance"], "projects/p/locations/z/instances/fs1"; got != want {
		t.Errorf("sourceInstance = %v, want %v", got, want)
	}
	if _, ok := body["location"]; ok {
		t.Error("location addresses the resource in the URL and must not be a body field")
	}
	if _, ok := body["name"]; !ok {
		t.Error("name must survive: base.Create reads the create id out of it")
	}

	out := backupResponse(map[string]interface{}{
		"name":           "projects/p/locations/z/backups/b1",
		"sourceInstance": "projects/p/locations/z/instances/fs1",
	}, base.TransformContext{})
	if out["name"] != "b1" || out["location"] != "z" || out["sourceInstance"] != "fs1" {
		t.Errorf("got %+v", out)
	}
}

// A full instance path written by hand must not be expanded a second time.
func TestBackupKeepsFullSourcePath(t *testing.T) {
	body, err := backupRequest(map[string]interface{}{
		"location":       "r",
		"sourceInstance": "projects/other/locations/z/instances/fs9",
	}, base.TransformContext{Project: "p"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := body["sourceInstance"], "projects/other/locations/z/instances/fs9"; got != want {
		t.Errorf("sourceInstance = %v, want %v", got, want)
	}
}

// instance and location live only in the path, but a forma declares both.
func TestSnapshotResponseRecoversParent(t *testing.T) {
	out := snapshotResponseTransformer(map[string]interface{}{
		"name": "projects/p/locations/z/instances/fs1/snapshots/s1",
	}, base.TransformContext{})
	if out["name"] != "s1" || out["location"] != "z" || out["instance"] != "fs1" {
		t.Errorf("got %+v", out)
	}

	// An instance's path must not be read as a snapshot's.
	out = snapshotResponseTransformer(map[string]interface{}{
		"name": "projects/p/locations/z/instances/fs1",
	}, base.TransformContext{})
	if out["name"] != "projects/p/locations/z/instances/fs1" {
		t.Errorf("foreign collection must be left alone: %+v", out)
	}
}
