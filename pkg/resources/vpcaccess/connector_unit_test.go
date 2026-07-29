// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package vpcaccess

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestPathBuilder(t *testing.T) {
	ctx := base.PathContext{Project: "p", Location: "us-central1", ResourceType: "connectors"}
	if got := vpcAccessPathBuilder(ctx); got != "/projects/p/locations/us-central1/connectors" {
		t.Errorf("collection path = %q", got)
	}
	ctx.ResourceName = "conn1"
	if got := vpcAccessPathBuilder(ctx); got != "/projects/p/locations/us-central1/connectors/conn1" {
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
		"name": "projects/p/locations/us-central1/connectors/conn1",
	}); got != "" {
		t.Errorf("resource name should not be treated as op: %q", got)
	}
}

func TestNativeIDFromOperationContext(t *testing.T) {
	// Async create: response is an Operation; native ID built from context.
	ctx := base.PathContext{Project: "p", Location: "us-central1", ResourceType: "connectors", ResourceName: "conn1"}
	got := extractVpcAccessNativeID(
		map[string]interface{}{"name": "projects/p/locations/us-central1/operations/op9", "done": false}, ctx)
	if got != "projects/p/locations/us-central1/connectors/conn1" {
		t.Errorf("native id = %q", got)
	}
}

func TestNativeIDFromResourceResponse(t *testing.T) {
	// get response: no ResourceName in context; native ID from full-path "name".
	ctx := base.PathContext{}
	got := extractVpcAccessNativeID(
		map[string]interface{}{"name": "projects/p/locations/us-central1/connectors/conn1"}, ctx)
	if got != "projects/p/locations/us-central1/connectors/conn1" {
		t.Errorf("native id = %q", got)
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

func TestConnectorRegistered(t *testing.T) {
	for _, op := range []resource.Operation{
		resource.OperationCreate, resource.OperationRead,
		resource.OperationDelete, resource.OperationList,
	} {
		if !registry.HasProvisioner(ConnectorResourceType, op) {
			t.Errorf("%s not registered for %v", ConnectorResourceType, op)
		}
	}
}
