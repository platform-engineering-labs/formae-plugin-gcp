// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package spanner

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestPathBuilder(t *testing.T) {
	ctx := base.PathContext{Project: "p", ResourceType: "instances"}
	if got := spannerPathBuilder(ctx); got != "/projects/p/instances" {
		t.Errorf("collection path = %q", got)
	}
	ctx.ResourceName = "inst1"
	if got := spannerPathBuilder(ctx); got != "/projects/p/instances/inst1" {
		t.Errorf("resource path = %q", got)
	}
}

// TestInstanceBodyBuilder proves the create body is reshaped into the
// {instanceId, instance:{...}} wrapper: id lifted to a sibling instanceId,
// name dropped from the inner object, and all other props nested under instance.
func TestInstanceBodyBuilder(t *testing.T) {
	props := map[string]interface{}{
		"name":            "my-instance",
		"config":          "projects/p/instanceConfigs/regional-us-central1",
		"displayName":     "My Instance",
		"processingUnits": float64(100),
		"labels":          map[string]interface{}{"env": "test"},
	}

	body, err := spannerInstanceBodyBuilder(props, base.TransformContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The id must be a sibling BODY field, not inside the instance.
	if got, ok := body["instanceId"].(string); !ok || got != "my-instance" {
		t.Errorf("instanceId = %v, want %q", body["instanceId"], "my-instance")
	}

	instance, ok := body["instance"].(map[string]interface{})
	if !ok {
		t.Fatalf("instance wrapper missing or wrong type: %T", body["instance"])
	}

	// name must NOT leak into the inner instance object.
	if _, present := instance["name"]; present {
		t.Errorf("inner instance should not contain 'name': %v", instance)
	}

	// Remaining declared fields must be nested under instance unchanged.
	if instance["config"] != "projects/p/instanceConfigs/regional-us-central1" {
		t.Errorf("instance.config = %v", instance["config"])
	}
	if instance["displayName"] != "My Instance" {
		t.Errorf("instance.displayName = %v", instance["displayName"])
	}
	if instance["processingUnits"] != float64(100) {
		t.Errorf("instance.processingUnits = %v", instance["processingUnits"])
	}
	if _, ok := instance["labels"].(map[string]interface{}); !ok {
		t.Errorf("instance.labels missing or wrong type: %v", instance["labels"])
	}

	// The top-level body has exactly the two expected keys.
	if len(body) != 2 {
		t.Errorf("body should have exactly {instanceId, instance}, got %v", body)
	}
}

func TestOperationName(t *testing.T) {
	// A create response is an Operation.
	if got := extractOperationName(map[string]interface{}{
		"name": "projects/p/instances/inst1/operations/op9",
	}); got != "projects/p/instances/inst1/operations/op9" {
		t.Errorf("op name = %q", got)
	}
	// A direct resource response is NOT an operation.
	if got := extractOperationName(map[string]interface{}{
		"name": "projects/p/instances/inst1",
	}); got != "" {
		t.Errorf("resource name should not be treated as op: %q", got)
	}
	// A delete returns Empty ({}) — no name, no operation.
	if got := extractOperationName(map[string]interface{}{}); got != "" {
		t.Errorf("empty delete response should yield no op: %q", got)
	}
}

func TestNativeIDFromOperationContext(t *testing.T) {
	// Async create: response is an Operation; native ID built from context.
	ctx := base.PathContext{Project: "p", ResourceType: "instances", ResourceName: "inst1"}
	got := extractSpannerNativeID(
		map[string]interface{}{"name": "projects/p/instances/inst1/operations/op9", "done": false}, ctx)
	if got != "projects/p/instances/inst1" {
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

func TestInstanceRegistered(t *testing.T) {
	for _, op := range []resource.Operation{
		resource.OperationCreate, resource.OperationRead,
		resource.OperationDelete, resource.OperationList,
		resource.OperationCheckStatus,
	} {
		if !registry.HasProvisioner(InstanceResourceType, op) {
			t.Errorf("%s not registered for %v", InstanceResourceType, op)
		}
	}
}
