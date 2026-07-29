// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package essentialcontacts

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestPathBuilder(t *testing.T) {
	ctx := base.PathContext{Project: "p", ResourceType: "contacts"}
	if got := essentialContactsPathBuilder(ctx); got != "/projects/p/contacts" {
		t.Errorf("collection path = %q", got)
	}
	ctx.ResourceName = "123"
	if got := essentialContactsPathBuilder(ctx); got != "/projects/p/contacts/123" {
		t.Errorf("resource path = %q", got)
	}
}

func TestNativeIDExtractor(t *testing.T) {
	ctx := base.PathContext{Project: "p", ResourceType: "contacts"}

	// Server-assigned name: create response carries the full path in "name".
	got := extractEssentialContactsNativeID(
		map[string]interface{}{"name": "projects/p/contacts/123"}, ctx)
	if got != "projects/p/contacts/123" {
		t.Errorf("native id from response = %q", got)
	}

	// Fallback: build from context when the response omits "name".
	ctx.ResourceName = "456"
	got = extractEssentialContactsNativeID(map[string]interface{}{}, ctx)
	if got != "projects/p/contacts/456" {
		t.Errorf("native id from context = %q", got)
	}
}

func TestNativeIDParser(t *testing.T) {
	ctx, err := parseEssentialContactsNativeID("projects/p/contacts/123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.Project != "p" || ctx.ResourceType != "contacts" || ctx.ResourceName != "123" {
		t.Errorf("parsed context = %+v", ctx)
	}
	if _, err := parseEssentialContactsNativeID("garbage"); err == nil {
		t.Errorf("expected error for invalid native ID")
	}
}

func TestShortNameRoundTrip(t *testing.T) {
	// The server-assigned full-path name is shortened to its id for stored state.
	out := base.ShortNameResponseTransformer.Transform(
		map[string]interface{}{"name": "projects/p/contacts/123"}, base.TransformContext{})
	if out["name"] != "123" {
		t.Errorf("short name = %v", out["name"])
	}
}

func TestContactRegistered(t *testing.T) {
	for _, op := range []resource.Operation{
		resource.OperationCreate, resource.OperationRead,
		resource.OperationDelete, resource.OperationList,
		resource.OperationCheckStatus,
	} {
		if !registry.HasProvisioner(ContactResourceType, op) {
			t.Errorf("%s not registered for %v", ContactResourceType, op)
		}
	}
	// Update is intentionally unsupported.
	if registry.HasProvisioner(ContactResourceType, resource.OperationUpdate) {
		t.Errorf("%s should not be registered for Update", ContactResourceType)
	}
}
