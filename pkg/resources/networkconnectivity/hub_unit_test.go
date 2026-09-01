// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package networkconnectivity

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestPathBuilderPinsGlobal(t *testing.T) {
	// A hub is global; the path builder must always emit locations/global.
	ctx := base.PathContext{Project: "p", ResourceType: "hubs"}
	if got := networkConnectivityPathBuilder(ctx); got != "/projects/p/locations/global/hubs" {
		t.Errorf("collection path = %q", got)
	}
	ctx.ResourceName = "hub1"
	if got := networkConnectivityPathBuilder(ctx); got != "/projects/p/locations/global/hubs/hub1" {
		t.Errorf("resource path = %q", got)
	}
}

func TestPathBuilderNoRegionLeak(t *testing.T) {
	// A configured region/location must NOT leak into the hub path — the
	// resource is always global.
	ctx := base.PathContext{
		Project:      "p",
		Region:       "europe-west1",
		Location:     "europe-west1",
		Zone:         "europe-west1-b",
		ResourceType: "hubs",
		ResourceName: "hub1",
	}
	got := networkConnectivityPathBuilder(ctx)
	want := "/projects/p/locations/global/hubs/hub1"
	if got != want {
		t.Errorf("region leaked into path: got %q, want %q", got, want)
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
		"name": "projects/p/locations/global/hubs/hub1",
	}); got != "" {
		t.Errorf("resource name should not be treated as op: %q", got)
	}
}

func TestNativeIDFromOperationContextPinsGlobal(t *testing.T) {
	// Async create: response is an Operation; native ID is built from context
	// and must be pinned to global even when a region is configured.
	ctx := base.PathContext{Project: "p", Location: "europe-west1", ResourceType: "hubs", ResourceName: "hub1"}
	got := extractNetworkConnectivityNativeID(
		map[string]interface{}{"name": "projects/p/locations/global/operations/op9", "done": false}, ctx)
	if got != "projects/p/locations/global/hubs/hub1" {
		t.Errorf("native id = %q", got)
	}
}

func TestNativeIDFromMetadataTarget(t *testing.T) {
	// No ResourceName in context: fall back to the operation's metadata.target.
	got := extractNetworkConnectivityNativeID(map[string]interface{}{
		"name":     "projects/p/locations/global/operations/op9",
		"metadata": map[string]interface{}{"target": "projects/p/locations/global/hubs/hub2"},
	}, base.PathContext{})
	if got != "projects/p/locations/global/hubs/hub2" {
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

func TestHubRegistered(t *testing.T) {
	for _, op := range []resource.Operation{
		resource.OperationCreate, resource.OperationRead,
		resource.OperationDelete, resource.OperationList,
		resource.OperationCheckStatus,
	} {
		if !registry.HasProvisioner(HubResourceType, op) {
			t.Errorf("%s not registered for %v", HubResourceType, op)
		}
	}
}

// The two global additions in this batch get the same guarantee hubs have: a
// configured region must not reach their URL.
func TestPathBuilderKeepsNewGlobalTypesGlobal(t *testing.T) {
	for _, rt := range []string{"internalRanges", "policyBasedRoutes"} {
		ctx := base.PathContext{
			Project:      "p",
			Region:       "europe-west1",
			Location:     "europe-west1",
			ResourceType: rt,
			ResourceName: "x",
		}
		got := networkConnectivityPathBuilder(ctx)
		want := "/projects/p/locations/global/" + rt + "/x"
		if got != want {
			t.Errorf("region leaked into %s path: got %q, want %q", rt, got, want)
		}
	}
}

// A service connection policy is the one regional type here, so the opposite
// has to hold: pinning it to global would address the wrong collection and the
// resource would read back as absent.
func TestPathBuilderKeepsPolicyRegional(t *testing.T) {
	ctx := base.PathContext{
		Project:      "p",
		Region:       "europe-central2",
		Location:     "europe-central2",
		ResourceType: "serviceConnectionPolicies",
		ResourceName: "x",
	}
	got := networkConnectivityPathBuilder(ctx)
	want := "/projects/p/locations/europe-central2/serviceConnectionPolicies/x"
	if got != want {
		t.Errorf("regional type was not regional: got %q, want %q", got, want)
	}
}

// With no location at all the regional type still has to build a usable path
// rather than an empty segment.
func TestPathBuilderFallsBackToGlobal(t *testing.T) {
	ctx := base.PathContext{Project: "p", ResourceType: "serviceConnectionPolicies", ResourceName: "x"}
	if got, want := networkConnectivityPathBuilder(ctx), "/projects/p/locations/global/serviceConnectionPolicies/x"; got != want {
		t.Errorf("empty location: got %q, want %q", got, want)
	}
}
