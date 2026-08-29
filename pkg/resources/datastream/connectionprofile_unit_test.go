// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package datastream

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestPathBuilder(t *testing.T) {
	ctx := base.PathContext{Project: "p", Location: "us-central1", ResourceType: "connectionProfiles"}
	// A create carries force=true: the profile is validated against its source
	// inside the create operation, and a failed validation leaves the profile
	// created but reported as failed.
	if got := datastreamPathBuilder(ctx); got != "/projects/p/locations/us-central1/connectionProfiles?force=true" {
		t.Errorf("collection path = %q", got)
	}
	// Listing takes no force.
	listCtx := ctx
	listCtx.IsList = true
	if got := datastreamPathBuilder(listCtx); got != "/projects/p/locations/us-central1/connectionProfiles" {
		t.Errorf("list path = %q", got)
	}
	ctx.ResourceName = "cp1"
	if got := datastreamPathBuilder(ctx); got != "/projects/p/locations/us-central1/connectionProfiles/cp1" {
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
		"name": "projects/p/locations/us-central1/connectionProfiles/cp1",
	}); got != "" {
		t.Errorf("resource name should not be treated as op: %q", got)
	}
}

func TestNativeIDFromOperationContext(t *testing.T) {
	// Async create: response is an Operation; native ID built from context.
	ctx := base.PathContext{Project: "p", Location: "us-central1", ResourceType: "connectionProfiles", ResourceName: "cp1"}
	got := extractDatastreamNativeID(
		map[string]interface{}{"name": "projects/p/locations/us-central1/operations/op9", "done": false}, ctx)
	if got != "projects/p/locations/us-central1/connectionProfiles/cp1" {
		t.Errorf("native id = %q", got)
	}
}

func TestNativeIDFromMetadataTarget(t *testing.T) {
	// No ResourceName in context: fall back to the operation metadata.target.
	ctx := base.PathContext{Project: "p", Location: "us-central1", ResourceType: "connectionProfiles"}
	got := extractDatastreamNativeID(map[string]interface{}{
		"name": "projects/p/locations/us-central1/operations/op9",
		"metadata": map[string]interface{}{
			"target": "projects/p/locations/us-central1/connectionProfiles/cp1",
		},
	}, ctx)
	if got != "projects/p/locations/us-central1/connectionProfiles/cp1" {
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

func TestConnectionProfileRegistered(t *testing.T) {
	for _, op := range []resource.Operation{
		resource.OperationCreate, resource.OperationRead,
		resource.OperationDelete, resource.OperationList,
	} {
		if !registry.HasProvisioner(ConnectionProfileResourceType, op) {
			t.Errorf("%s not registered for %v", ConnectionProfileResourceType, op)
		}
	}
}

// Datastream validates a stream against its source on create; a conformance
// source points at a host that does not answer, so create must send force.
func TestStreamCreateForcesPastValidation(t *testing.T) {
	create := datastreamPathBuilder(base.PathContext{
		Project: "p", Location: "eu", ResourceType: "streams",
	})
	if want := "/projects/p/locations/eu/streams?force=true"; create != want {
		t.Errorf("create = %q, want %q", create, want)
	}

	// force is only correct on create. List builds the same collection URL.
	list := datastreamPathBuilder(base.PathContext{
		Project: "p", Location: "eu", ResourceType: "streams", IsList: true,
	})
	if want := "/projects/p/locations/eu/streams"; list != want {
		t.Errorf("list = %q, want %q", list, want)
	}

	// A connection profile is validated against its source inside the create
	// operation, so it needs force too.
	cp := datastreamPathBuilder(base.PathContext{
		Project: "p", Location: "eu", ResourceType: "connectionProfiles",
	})
	if want := "/projects/p/locations/eu/connectionProfiles?force=true"; cp != want {
		t.Errorf("connectionProfiles = %q, want %q", cp, want)
	}

	// A collection with no validation step must not grow it.
	other := datastreamPathBuilder(base.PathContext{
		Project: "p", Location: "eu", ResourceType: "privateConnections",
	})
	if want := "/projects/p/locations/eu/privateConnections"; other != want {
		t.Errorf("privateConnections = %q, want %q", other, want)
	}
}

func TestDatastreamNestedPathAndNativeID(t *testing.T) {
	got := datastreamPathBuilder(base.PathContext{
		Project: "p", Location: "eu",
		ParentType: "privateConnections", ParentResource: "pc1",
		ResourceType: "routes", ResourceName: "r1",
	})
	if want := "/projects/p/locations/eu/privateConnections/pc1/routes/r1"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	ctx, err := parseDatastreamNativeID("projects/p/locations/eu/privateConnections/pc1/routes/r1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.ParentResource != "pc1" || ctx.ResourceType != "routes" || ctx.ResourceName != "r1" {
		t.Errorf("nested parse wrong: %+v", ctx)
	}
	if _, err := parseDatastreamNativeID("projects/p/locations/eu/streams"); err == nil {
		t.Error("a collection path is not a resource and must be rejected")
	}
}

// A forma passes resolvables that resolve to bare profile names; the API wants
// full paths. Expand on write, shorten on read.
func TestStreamProfileRoundTrip(t *testing.T) {
	body, err := streamRequest(map[string]interface{}{
		"name": "s1",
		"sourceConfig": map[string]interface{}{
			"sourceConnectionProfile": "src",
		},
		"destinationConfig": map[string]interface{}{
			"destinationConnectionProfile": "dst",
		},
	}, base.TransformContext{Project: "p", Location: "eu"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	src := body["sourceConfig"].(map[string]interface{})
	if got, want := src["sourceConnectionProfile"], "projects/p/locations/eu/connectionProfiles/src"; got != want {
		t.Errorf("source = %v, want %v", got, want)
	}
	dst := body["destinationConfig"].(map[string]interface{})
	if got, want := dst["destinationConnectionProfile"], "projects/p/locations/eu/connectionProfiles/dst"; got != want {
		t.Errorf("destination = %v, want %v", got, want)
	}

	out := streamResponse(map[string]interface{}{
		"name":              "projects/p/locations/eu/streams/s1",
		"sourceConfig":      map[string]interface{}{"sourceConnectionProfile": src["sourceConnectionProfile"]},
		"destinationConfig": map[string]interface{}{"destinationConnectionProfile": dst["destinationConnectionProfile"]},
	}, base.TransformContext{})
	if got := out["sourceConfig"].(map[string]interface{})["sourceConnectionProfile"]; got != "src" {
		t.Errorf("source = %v, want src", got)
	}
	if got := out["destinationConfig"].(map[string]interface{})["destinationConnectionProfile"]; got != "dst" {
		t.Errorf("destination = %v, want dst", got)
	}
}

// privateConnection lives only in the path, but a forma declares it.
func TestRouteResponseRecoversParent(t *testing.T) {
	out := routeResponse(map[string]interface{}{
		"name": "projects/p/locations/eu/privateConnections/pc1/routes/r1",
	}, base.TransformContext{})
	if out["privateConnection"] != "pc1" {
		t.Errorf("got %+v", out)
	}
}

// Discovery lists with no parent. Datastream accepts "-" in the
// private-connection position, so a parentless list of routes must use it
// rather than emitting a path with no parent - which 404s, and left routes
// undiscoverable.
func TestRouteListUsesPrivateConnectionWildcard(t *testing.T) {
	got := datastreamPathBuilder(base.PathContext{
		Project: "p", Location: "eu", ResourceType: "routes", IsList: true,
	})
	if want := "/projects/p/locations/eu/privateConnections/-/routes"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// A named parent still wins.
	got = datastreamPathBuilder(base.PathContext{
		Project: "p", Location: "eu",
		ParentType: "privateConnections", ParentResource: "pc1",
		ResourceType: "routes", IsList: true,
	})
	if want := "/projects/p/locations/eu/privateConnections/pc1/routes"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// A top-level collection must not grow a wildcard.
	got = datastreamPathBuilder(base.PathContext{
		Project: "p", Location: "eu", ResourceType: "privateConnections", IsList: true,
	})
	if want := "/projects/p/locations/eu/privateConnections"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// And the wildcard is a list-only concern: a create must not get one.
	got = datastreamPathBuilder(base.PathContext{
		Project: "p", Location: "eu", ResourceType: "routes",
	})
	if want := "/projects/p/locations/eu/routes"; got != want {
		t.Errorf("create path = %q, want %q", got, want)
	}
}
