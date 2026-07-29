// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package kms

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestPathBuilder(t *testing.T) {
	ctx := base.PathContext{Project: "p", Location: "us-central1", ResourceType: "keyRings"}
	if got := kmsPathBuilder(ctx); got != "/projects/p/locations/us-central1/keyRings" {
		t.Errorf("collection path = %q", got)
	}
	ctx.ResourceName = "ring1"
	if got := kmsPathBuilder(ctx); got != "/projects/p/locations/us-central1/keyRings/ring1" {
		t.Errorf("resource path = %q", got)
	}
}

func TestNativeIDFromResponse(t *testing.T) {
	// Sync create/get: response carries the full path in "name".
	ctx := base.PathContext{Project: "p", Location: "us-central1", ResourceType: "keyRings", ResourceName: "ring1"}
	got := extractKMSNativeID(
		map[string]interface{}{"name": "projects/p/locations/us-central1/keyRings/ring1"}, ctx)
	if got != "projects/p/locations/us-central1/keyRings/ring1" {
		t.Errorf("native id from response = %q", got)
	}
}

func TestNativeIDFromContext(t *testing.T) {
	// Fallback: build the path from context when the response lacks a name.
	ctx := base.PathContext{Project: "p", Location: "us-central1", ResourceType: "keyRings", ResourceName: "ring1"}
	got := extractKMSNativeID(map[string]interface{}{}, ctx)
	if got != "projects/p/locations/us-central1/keyRings/ring1" {
		t.Errorf("native id from context = %q", got)
	}
}

func TestNativeIDParseRoundTrip(t *testing.T) {
	// Default FullPathFormat parser must recover the location-scoped context.
	ctx, err := base.ParseNativeID(KMSNativeID, "projects/p/locations/us-central1/keyRings/ring1")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if ctx.Project != "p" || ctx.Location != "us-central1" ||
		ctx.ResourceType != "keyRings" || ctx.ResourceName != "ring1" {
		t.Errorf("parsed context = %+v", ctx)
	}
}

func TestKeyRingRegistered(t *testing.T) {
	// KeyRings support only Create/Read/List (+ CheckStatus): no delete/update.
	for _, op := range []resource.Operation{
		resource.OperationCreate,
		resource.OperationRead,
		resource.OperationList,
		resource.OperationCheckStatus,
	} {
		if !registry.HasProvisioner(KeyRingResourceType, op) {
			t.Errorf("%s not registered for %v", KeyRingResourceType, op)
		}
	}
	// Delete and Update are intentionally NOT registered (unsupported by API).
	for _, op := range []resource.Operation{
		resource.OperationDelete,
		resource.OperationUpdate,
	} {
		if registry.HasProvisioner(KeyRingResourceType, op) {
			t.Errorf("%s should NOT be registered for %v (KMS KeyRings cannot be %v)", KeyRingResourceType, op, op)
		}
	}
}
