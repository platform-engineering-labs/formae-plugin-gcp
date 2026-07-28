// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package servicedirectory

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestPathBuilder(t *testing.T) {
	ctx := base.PathContext{Project: "p", Location: "us-central1", ResourceType: "namespaces"}
	if got := serviceDirectoryPathBuilder(ctx); got != "/projects/p/locations/us-central1/namespaces" {
		t.Errorf("collection path = %q", got)
	}
	ctx.ResourceName = "ns1"
	if got := serviceDirectoryPathBuilder(ctx); got != "/projects/p/locations/us-central1/namespaces/ns1" {
		t.Errorf("resource path = %q", got)
	}
}

func TestNativeIDFromResponse(t *testing.T) {
	// Synchronous create: the response is the Namespace itself with a full-path name.
	got := extractServiceDirectoryNativeID(
		map[string]interface{}{"name": "projects/p/locations/us-central1/namespaces/ns1"},
		base.PathContext{})
	if got != "projects/p/locations/us-central1/namespaces/ns1" {
		t.Errorf("native id from response = %q", got)
	}
}

func TestNativeIDFromContext(t *testing.T) {
	// Fallback: build the path from context when the response omits "name".
	ctx := base.PathContext{Project: "p", Location: "us-central1", ResourceType: "namespaces", ResourceName: "ns1"}
	got := extractServiceDirectoryNativeID(map[string]interface{}{}, ctx)
	if got != "projects/p/locations/us-central1/namespaces/ns1" {
		t.Errorf("native id from context = %q", got)
	}
}

func TestNativeIDRoundTrip(t *testing.T) {
	// The default FullPathFormat parser must recover project/location/name.
	ctx, err := base.ParseNativeID(ServiceDirectoryNativeID, "projects/p/locations/us-central1/namespaces/ns1")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if ctx.Project != "p" || ctx.Location != "us-central1" || ctx.ResourceName != "ns1" {
		t.Errorf("parsed ctx = %+v", ctx)
	}
}

func TestNamespaceRegistered(t *testing.T) {
	for _, op := range []resource.Operation{
		resource.OperationCreate, resource.OperationRead,
		resource.OperationDelete, resource.OperationList,
		resource.OperationCheckStatus,
	} {
		if !registry.HasProvisioner(NamespaceResourceType, op) {
			t.Errorf("%s not registered for %v", NamespaceResourceType, op)
		}
	}
}
