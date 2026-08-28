// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package certificateauthority

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestPathBuilder(t *testing.T) {
	ctx := base.PathContext{Project: "p", Location: "us-central1", ResourceType: "caPools"}
	if got := certificateAuthorityPathBuilder(ctx); got != "/projects/p/locations/us-central1/caPools" {
		t.Errorf("collection path = %q", got)
	}
	ctx.ResourceName = "pool1"
	if got := certificateAuthorityPathBuilder(ctx); got != "/projects/p/locations/us-central1/caPools/pool1" {
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
		"name": "projects/p/locations/us-central1/caPools/pool1",
	}); got != "" {
		t.Errorf("resource name should not be treated as op: %q", got)
	}
}

func TestNativeIDFromOperationContext(t *testing.T) {
	// Async create: response is an Operation; native ID built from context.
	ctx := base.PathContext{Project: "p", Location: "us-central1", ResourceType: "caPools", ResourceName: "pool1"}
	got := extractCaPoolNativeID(
		map[string]interface{}{"name": "projects/p/locations/us-central1/operations/op9", "done": false}, ctx)
	if got != "projects/p/locations/us-central1/caPools/pool1" {
		t.Errorf("native id = %q", got)
	}
}

func TestNativeIDFromMetadataTarget(t *testing.T) {
	// No context ResourceName: fall back to the operation's metadata.target.
	ctx := base.PathContext{Project: "p", Location: "us-central1", ResourceType: "caPools"}
	got := extractCaPoolNativeID(map[string]interface{}{
		"name": "projects/p/locations/us-central1/operations/op9",
		"metadata": map[string]interface{}{
			"target": "projects/p/locations/us-central1/caPools/pool1",
		},
	}, ctx)
	if got != "projects/p/locations/us-central1/caPools/pool1" {
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

func TestCaPoolRegistered(t *testing.T) {
	for _, op := range []resource.Operation{
		resource.OperationCreate, resource.OperationRead,
		resource.OperationDelete, resource.OperationList,
	} {
		if !registry.HasProvisioner(CaPoolResourceType, op) {
			t.Errorf("%s not registered for %v", CaPoolResourceType, op)
		}
	}
}

// A CA is addressed inside its pool; a pool and a template are not.
func TestPrivateCAPathBuilderNesting(t *testing.T) {
	got := certificateAuthorityPathBuilder(base.PathContext{
		Project: "p", Location: "eu",
		ParentType: "caPools", ParentResource: "pool1",
		ResourceType: "certificateAuthorities", ResourceName: "ca1",
	})
	if want := "/projects/p/locations/eu/caPools/pool1/certificateAuthorities/ca1"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	got = certificateAuthorityPathBuilder(base.PathContext{
		Project: "p", Location: "eu",
		ResourceType: "certificateTemplates", ResourceName: "t1",
	})
	if want := "/projects/p/locations/eu/certificateTemplates/t1"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrivateCANativeIDParser(t *testing.T) {
	ctx, err := parsePrivateCANativeID("projects/p/locations/eu/caPools/pool1/certificateAuthorities/ca1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.ParentType != "caPools" || ctx.ParentResource != "pool1" ||
		ctx.ResourceType != "certificateAuthorities" || ctx.ResourceName != "ca1" {
		t.Errorf("nested parse wrong: %+v", ctx)
	}

	ctx, err = parsePrivateCANativeID("projects/p/locations/eu/certificateTemplates/t1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.ResourceType != "certificateTemplates" || ctx.ResourceName != "t1" || ctx.Location != "eu" {
		t.Errorf("top-level parse wrong: %+v", ctx)
	}

	if _, err := parsePrivateCANativeID("projects/p/locations/eu/caPools"); err == nil {
		t.Error("a collection path is not a resource and must be rejected")
	}
}

// The API reports a full path and never reports location or caPool as fields,
// but a forma declares all three.
func TestCAResponseTransformer(t *testing.T) {
	out := caResponseTransformer(map[string]interface{}{
		"name": "projects/p/locations/eu/caPools/pool1/certificateAuthorities/ca1",
	}, base.TransformContext{})
	if out["name"] != "ca1" || out["location"] != "eu" || out["caPool"] != "pool1" {
		t.Errorf("got %+v", out)
	}

	// A pool's path must not be read as a CA's.
	out = caResponseTransformer(map[string]interface{}{
		"name": "projects/p/locations/eu/caPools/pool1",
	}, base.TransformContext{})
	if out["name"] != "projects/p/locations/eu/caPools/pool1" {
		t.Errorf("foreign collection must be left alone: %+v", out)
	}
}

// location and caPool address the resource in the URL; name must survive
// because base.Create reads the create id out of it.
func TestDropCAPathFields(t *testing.T) {
	body, err := dropCAPathFields(map[string]interface{}{
		"name": "ca1", "location": "eu", "caPool": "pool1", "lifetime": "1s",
	}, base.TransformContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, k := range []string{"location", "caPool"} {
		if _, ok := body[k]; ok {
			t.Errorf("%q must not be a body field", k)
		}
	}
	if body["name"] != "ca1" || body["lifetime"] != "1s" {
		t.Errorf("got %+v", body)
	}
}

// A CA that is merely DELETE-d sits tombstoned for 30 days, still billed.
func TestCADeleteSkipsGracePeriod(t *testing.T) {
	if caDeleteParams["skipGracePeriod"] != "true" {
		t.Error("skipGracePeriod must be set or a destroy leaves the CA billed for 30 days")
	}
	for _, k := range []string{"ignoreActiveCertificates", "ignoreDependentResources"} {
		if caDeleteParams[k] != "true" {
			t.Errorf("%s must be set so a CA that issued something still tears down", k)
		}
	}
}
