// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package storage

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// An object ACL is addressed by a bucket and an object at once. If the second
// property is not wired, both the create URL and the native ID collapse to the
// bucket-scoped form - which is a different resource that the API happily
// creates.
func TestObjectAclCarriesBothParents(t *testing.T) {
	def, ok := storageRegistry.Definitions[ObjectAccessControlResourceType]
	if !ok {
		t.Fatal("object access control is not registered")
	}
	parent := def.ResourceConfig.ParentResource
	if parent == nil {
		t.Fatal("object access control has no parent config")
	}
	if parent.PropertyName != "bucket" {
		t.Errorf("first parent property = %q, want bucket", parent.PropertyName)
	}
	if parent.SecondPropertyName != "object" {
		t.Errorf("second parent property = %q, want object", parent.SecondPropertyName)
	}
}

// The native ID has to keep the object segment: discovery reports
// b/{bucket}/o/{object}/acl/{entity}, and a create that reports
// b/{bucket}/acl/{entity} never matches it.
func TestObjectAclNativeIDKeepsObject(t *testing.T) {
	got := buildStorageNativeID("user-someone@example.com", base.PathContext{Project: "p", ResourceType: "acl", ParentResource: "bkt/conformance/acl-target.txt"})
	want := "b/bkt/o/conformance/acl-target.txt/acl/user-someone@example.com"
	if got != want {
		t.Errorf("native ID = %q, want %q", got, want)
	}
}
