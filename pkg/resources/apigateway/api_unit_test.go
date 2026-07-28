// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package apigateway

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestPathBuilder(t *testing.T) {
	// Location is always "global" for apis, even though it isn't set on the ctx.
	ctx := base.PathContext{Project: "p", ResourceType: "apis"}
	if got := apiGatewayPathBuilder(ctx); got != "/projects/p/locations/global/apis" {
		t.Errorf("collection path = %q", got)
	}
	ctx.ResourceName = "api1"
	if got := apiGatewayPathBuilder(ctx); got != "/projects/p/locations/global/apis/api1" {
		t.Errorf("resource path = %q", got)
	}
}

func TestPathBuilderIgnoresConfiguredLocation(t *testing.T) {
	// A region on the ctx must not leak into the path — apis are global-only.
	ctx := base.PathContext{Project: "p", Location: "us-central1", ResourceType: "apis", ResourceName: "api1"}
	if got := apiGatewayPathBuilder(ctx); got != "/projects/p/locations/global/apis/api1" {
		t.Errorf("path leaked region: %q", got)
	}
}

func TestOperationName(t *testing.T) {
	// A create/delete response is an Operation.
	if got := extractOperationName(map[string]interface{}{
		"name": "projects/p/locations/global/operations/op9",
	}); got != "projects/p/locations/global/operations/op9" {
		t.Errorf("op name = %q", got)
	}
	// A direct resource response is NOT an operation.
	if got := extractOperationName(map[string]interface{}{
		"name": "projects/p/locations/global/apis/api1",
	}); got != "" {
		t.Errorf("resource name should not be treated as op: %q", got)
	}
}

func TestNativeIDFromOperationContext(t *testing.T) {
	// Async create: response is an Operation; native ID built from context,
	// always with location "global".
	ctx := base.PathContext{Project: "p", ResourceType: "apis", ResourceName: "api1"}
	got := extractAPINativeID(
		map[string]interface{}{"name": "projects/p/locations/global/operations/op9", "done": false}, ctx)
	if got != "projects/p/locations/global/apis/api1" {
		t.Errorf("native id = %q", got)
	}
}

func TestNativeIDFromMetadataTarget(t *testing.T) {
	// With no ResourceName on ctx, fall back to the operation metadata target.
	got := extractAPINativeID(map[string]interface{}{
		"name":     "projects/p/locations/global/operations/op9",
		"metadata": map[string]interface{}{"target": "projects/p/locations/global/apis/api1"},
	}, base.PathContext{})
	if got != "projects/p/locations/global/apis/api1" {
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

func TestAPIRegistered(t *testing.T) {
	for _, op := range []resource.Operation{
		resource.OperationCreate, resource.OperationRead,
		resource.OperationDelete, resource.OperationList,
		resource.OperationCheckStatus,
	} {
		if !registry.HasProvisioner(APIResourceType, op) {
			t.Errorf("%s not registered for %v", APIResourceType, op)
		}
	}
}
