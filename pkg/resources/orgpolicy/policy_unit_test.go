// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package orgpolicy

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestPathBuilder(t *testing.T) {
	ctx := base.PathContext{Project: "p", ResourceType: "policies"}
	if got := orgPolicyPathBuilder(ctx); got != "/projects/p/policies" {
		t.Errorf("collection path = %q", got)
	}
	ctx.ResourceName = "iam.disableServiceAccountKeyCreation"
	if got := orgPolicyPathBuilder(ctx); got != "/projects/p/policies/iam.disableServiceAccountKeyCreation" {
		t.Errorf("resource path = %q", got)
	}
}

func TestNativeIDExtractor(t *testing.T) {
	// Sync create/read response carries the full path in "name" (project number).
	got := extractOrgPolicyNativeID(
		map[string]interface{}{"name": "projects/123/policies/iam.disableServiceAccountKeyCreation"},
		base.PathContext{Project: "123", ResourceType: "policies"})
	if got != "projects/123/policies/iam.disableServiceAccountKeyCreation" {
		t.Errorf("native id from response = %q", got)
	}
	// Fallback: rebuild from context when "name" is absent.
	got = extractOrgPolicyNativeID(
		map[string]interface{}{},
		base.PathContext{Project: "p", ResourceType: "policies", ResourceName: "compute.disableSerialPortAccess"})
	if got != "projects/p/policies/compute.disableSerialPortAccess" {
		t.Errorf("native id from context = %q", got)
	}
}

func TestNativeIDParser(t *testing.T) {
	ctx, err := parseOrgPolicyNativeID("projects/123/policies/iam.disableServiceAccountKeyCreation")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.Project != "123" || ctx.ResourceType != "policies" || ctx.ResourceName != "iam.disableServiceAccountKeyCreation" {
		t.Errorf("parsed ctx = %+v", ctx)
	}
	if _, err := parseOrgPolicyNativeID("projects/123/constraints/foo"); err == nil {
		t.Error("expected error for non-policy native ID")
	}
	if _, err := parseOrgPolicyNativeID("garbage"); err == nil {
		t.Error("expected error for malformed native ID")
	}
}

func TestExpandNameRequestTransformer(t *testing.T) {
	// Short constraint id is expanded to the full create-body path.
	out, err := expandNameRequestTransformer(
		map[string]interface{}{
			"name": "iam.disableServiceAccountKeyCreation",
			"spec": map[string]interface{}{"rules": []interface{}{map[string]interface{}{"enforce": true}}},
		},
		base.TransformContext{Project: "my-proj", Operation: resource.OperationCreate})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := out["name"]; got != "projects/my-proj/policies/iam.disableServiceAccountKeyCreation" {
		t.Errorf("expanded name = %q", got)
	}
	// spec must be preserved untouched.
	if _, ok := out["spec"]; !ok {
		t.Error("spec was dropped by transformer")
	}

	// Idempotent: an already-full path is left untouched.
	out, err = expandNameRequestTransformer(
		map[string]interface{}{"name": "projects/my-proj/policies/iam.disableServiceAccountKeyCreation"},
		base.TransformContext{Project: "my-proj", Operation: resource.OperationCreate})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := out["name"]; got != "projects/my-proj/policies/iam.disableServiceAccountKeyCreation" {
		t.Errorf("full path was rewritten = %q", got)
	}
}

func TestPolicyRegistered(t *testing.T) {
	for _, op := range []resource.Operation{
		resource.OperationCreate, resource.OperationRead,
		resource.OperationDelete, resource.OperationList,
	} {
		if !registry.HasProvisioner(PolicyResourceType, op) {
			t.Errorf("%s not registered for %v", PolicyResourceType, op)
		}
	}
	// Update is deferred and must NOT be registered.
	if registry.HasProvisioner(PolicyResourceType, resource.OperationUpdate) {
		t.Errorf("%s should not be registered for Update", PolicyResourceType)
	}
}
