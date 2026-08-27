// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package sql

import (
	"strings"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// delete and update address a user by query parameter against the collection
// URL, not by path - ".../users?name=x". The generic engine builds
// ".../users/{name}", which addresses nothing.
func TestUserCollectionURLPutsTheNameInTheQuery(t *testing.T) {
	p := &userProvisioner{}
	got, err := p.collectionURL("projects/proj/instances/inst/users/alice", true)
	if err != nil {
		t.Fatalf("collectionURL: %v", err)
	}
	want := SQLAPI.BaseURL + "/projects/proj/instances/inst/users?name=alice"
	if got != want {
		t.Errorf("url = %q, want %q", got, want)
	}
	if strings.Contains(got, "/users/alice") {
		t.Errorf("name must not be a path segment: %q", got)
	}
}

func TestUserCollectionURLWithoutIdentity(t *testing.T) {
	p := &userProvisioner{}
	got, err := p.collectionURL("projects/proj/instances/inst/users/alice", false)
	if err != nil {
		t.Fatalf("collectionURL: %v", err)
	}
	if got != SQLAPI.BaseURL+"/projects/proj/instances/inst/users" {
		t.Errorf("url = %q", got)
	}
}

// A native ID that does not name an instance cannot address a user, and must
// fail rather than build a URL against the project-level collection.
func TestUserCollectionURLRejectsNonUserNativeIDs(t *testing.T) {
	p := &userProvisioner{}
	for _, id := range []string{
		"projects/proj/instances/inst",
		"projects/proj/users/alice",
		"nonsense",
	} {
		if _, err := p.collectionURL(id, true); err == nil {
			t.Errorf("expected an error for %q", id)
		}
	}
}

// Every sqladmin mutation answers with an Operation, and base.Status polls it
// at projects/{project}/operations/{id}.
func TestUserOperationRequestID(t *testing.T) {
	p := &userProvisioner{}
	got := p.operationRequestID(
		map[string]interface{}{"name": "op-123"},
		"projects/proj/instances/inst/users/alice")
	if got != "projects/proj/operations/op-123" {
		t.Errorf("requestID = %q", got)
	}
	if got := p.operationRequestID(map[string]interface{}{}, "projects/proj/instances/inst/users/alice"); got != "" {
		t.Errorf("expected empty requestID with no operation, got %q", got)
	}
}

// The two overridden verbs must keep the custom provisioner; create and read
// must keep the generic one. registerUserOverrides is called from the package
// init in resources.go so the generic registration always lands first.
func TestUserOverridesAreRegistered(t *testing.T) {
	for _, op := range []resource.Operation{
		resource.OperationDelete, resource.OperationList,
	} {
		p := registry.Get(UserResourceType, op, nil)
		if _, ok := p.(*userProvisioner); !ok {
			t.Errorf("%v is %T, want *userProvisioner", op, p)
		}
	}
	for _, op := range []resource.Operation{resource.OperationCreate, resource.OperationRead} {
		if !registry.HasProvisioner(UserResourceType, op) {
			t.Errorf("%v not registered", op)
		}
		if _, ok := registry.Get(UserResourceType, op, nil).(*userProvisioner); ok {
			t.Errorf("%v should keep the generic provisioner", op)
		}
	}
}

// "project", "kind" and "etag" are echoed by sqladmin but describe the request
// rather than the user; leaving them in would read as properties nobody declared.
func TestUserResponseDropsEchoedFields(t *testing.T) {
	out := userResponseTransformer(map[string]interface{}{
		"name": "alice", "instance": "inst", "project": "proj", "kind": "sql#user", "etag": "x",
	}, base.TransformContext{})
	if out["name"] != "alice" || out["instance"] != "inst" {
		t.Errorf("user = %+v", out)
	}
	for _, k := range []string{"project", "kind", "etag"} {
		if _, present := out[k]; present {
			t.Errorf("%s should have been dropped: %+v", k, out)
		}
	}
}

// Every sqladmin mutation answers with an Operation whose targetLink points at
// the *instance*. A nested resource that took its native ID from there would be
// stored under the instance's id — two resources sharing one native ID — and the
// next sync would read the instance and reconcile the nested resource away as
// absent. This is what made the first cloudsql-user run fail at Sync with
// "Inventory should still contain exactly 1 resource after sync, got 0".
func TestNestedNativeIDIgnoresTheOperationTargetLink(t *testing.T) {
	operation := map[string]interface{}{
		"name":       "op-1",
		"targetLink": "https://sqladmin.googleapis.com/v1/projects/proj/instances/inst",
	}
	ctx := base.PathContext{
		Project: "proj", ParentType: "instances", ParentResource: "inst",
		ResourceType: "users", ResourceName: "alice",
	}
	if got := extractSQLNativeID(operation, ctx); got != "projects/proj/instances/inst/users/alice" {
		t.Errorf("native id = %q", got)
	}
}

// A top-level instance still takes its id from the operation, which is the only
// place it appears.
func TestTopLevelNativeIDStillUsesTargetLink(t *testing.T) {
	operation := map[string]interface{}{
		"targetLink": "https://sqladmin.googleapis.com/v1/projects/proj/instances/inst",
	}
	if got := extractSQLNativeID(operation, base.PathContext{Project: "proj"}); got != "projects/proj/instances/inst" {
		t.Errorf("native id = %q", got)
	}
}
