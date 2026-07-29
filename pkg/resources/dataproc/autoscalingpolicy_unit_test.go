// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package dataproc

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestPathBuilder(t *testing.T) {
	ctx := base.PathContext{Project: "p", Region: "us-central1", ResourceType: "autoscalingPolicies"}
	if got := dataprocPathBuilder(ctx); got != "/projects/p/regions/us-central1/autoscalingPolicies" {
		t.Errorf("collection path = %q", got)
	}
	ctx.ResourceName = "policy1"
	if got := dataprocPathBuilder(ctx); got != "/projects/p/regions/us-central1/autoscalingPolicies/policy1" {
		t.Errorf("resource path = %q", got)
	}
}

func TestNativeIDExtractorFromResponse(t *testing.T) {
	// Sync create echoes the full resource path in "name".
	ctx := base.PathContext{Project: "p", Region: "us-central1", ResourceType: "autoscalingPolicies"}
	got := extractDataprocNativeID(map[string]interface{}{
		"name": "projects/p/regions/us-central1/autoscalingPolicies/policy1",
		"id":   "policy1",
	}, ctx)
	if got != "projects/p/regions/us-central1/autoscalingPolicies/policy1" {
		t.Errorf("native id from response = %q", got)
	}
}

func TestNativeIDExtractorFromContext(t *testing.T) {
	// When the response lacks a full-path name, reconstruct from context + id.
	ctx := base.PathContext{Project: "p", Region: "us-central1", ResourceType: "autoscalingPolicies", ResourceName: "policy1"}
	got := extractDataprocNativeID(map[string]interface{}{"id": "policy1"}, ctx)
	if got != "projects/p/regions/us-central1/autoscalingPolicies/policy1" {
		t.Errorf("native id from context = %q", got)
	}
}

func TestParseNativeID(t *testing.T) {
	ctx, err := parseDataprocNativeID("projects/p/regions/us-central1/autoscalingPolicies/policy1")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if ctx.Project != "p" || ctx.Region != "us-central1" ||
		ctx.ResourceType != "autoscalingPolicies" || ctx.ResourceName != "policy1" {
		t.Errorf("parsed ctx = %+v", ctx)
	}
	if _, err := parseDataprocNativeID("projects/p/locations/us-central1/autoscalingPolicies/policy1"); err == nil {
		t.Errorf("expected error for non-region-scoped native ID")
	}
}

func TestIDRequestTransformer(t *testing.T) {
	// name (the user-declared short id) must become body "id"; "name" must be
	// dropped because it is output-only on the API side.
	out, err := dataprocIDRequestTransformer.Transform(map[string]interface{}{
		"name":         "policy1",
		"workerConfig": map[string]interface{}{"maxInstances": 2},
	}, base.TransformContext{})
	if err != nil {
		t.Fatalf("transform error: %v", err)
	}
	if out["id"] != "policy1" {
		t.Errorf("id = %v, want policy1", out["id"])
	}
	if _, ok := out["name"]; ok {
		t.Errorf("name should be removed from body, got %v", out["name"])
	}
	if _, ok := out["workerConfig"]; !ok {
		t.Errorf("workerConfig should be preserved")
	}
}

func TestShortNameResponseTransformer(t *testing.T) {
	// The API returns "name" as the full path; the response transformer must
	// shorten it to the identifier the user declared.
	out := base.ShortNameResponseTransformer.Transform(map[string]interface{}{
		"name": "projects/p/regions/us-central1/autoscalingPolicies/policy1",
	}, base.TransformContext{})
	if out["name"] != "policy1" {
		t.Errorf("short name = %v, want policy1", out["name"])
	}
}

func TestSyncOperationConfig(t *testing.T) {
	if !DataprocOperations.Synchronous {
		t.Errorf("autoscalingPolicies operations should be synchronous")
	}
	if done, err := DataprocOperations.OperationStatusChecker(nil); !done || err != nil {
		t.Errorf("sync status checker = (%v,%v), want (true,nil)", done, err)
	}
}

func TestAutoscalingPolicyRegistered(t *testing.T) {
	for _, op := range []resource.Operation{
		resource.OperationCreate, resource.OperationRead,
		resource.OperationDelete, resource.OperationList,
		resource.OperationCheckStatus,
	} {
		if !registry.HasProvisioner(AutoscalingPolicyResourceType, op) {
			t.Errorf("%s not registered for %v", AutoscalingPolicyResourceType, op)
		}
	}
	// Update is intentionally not supported yet.
	if registry.HasProvisioner(AutoscalingPolicyResourceType, resource.OperationUpdate) {
		t.Errorf("%s should not register Update", AutoscalingPolicyResourceType)
	}
}
