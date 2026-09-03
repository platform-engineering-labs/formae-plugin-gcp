//go:build unit

// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package networkservices

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// Meshes and service LB policies live under locations/global whatever region
// the target names; gateways live under the target's region. Getting this wrong
// is a 404 on every call, so both halves are pinned.
func TestPathBuilderScopesByCollection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		ctx      base.PathContext
		wantPath string
	}{
		{
			name:     "mesh collection is global",
			ctx:      base.PathContext{Project: "p", ResourceType: "meshes"},
			wantPath: "/projects/p/locations/global/meshes",
		},
		{
			name:     "mesh resource is global",
			ctx:      base.PathContext{Project: "p", ResourceType: "meshes", ResourceName: "m1"},
			wantPath: "/projects/p/locations/global/meshes/m1",
		},
		{
			name:     "service LB policy is global",
			ctx:      base.PathContext{Project: "p", ResourceType: "serviceLbPolicies", ResourceName: "lb1"},
			wantPath: "/projects/p/locations/global/serviceLbPolicies/lb1",
		},
		{
			name: "gateway takes the target's location",
			ctx: base.PathContext{
				Project: "p", Location: "europe-central2",
				ResourceType: "gateways", ResourceName: "g1",
			},
			wantPath: "/projects/p/locations/europe-central2/gateways/g1",
		},
		{
			// base.ScopeGlobal clears Location, so a gateway with no location
			// arrives here empty rather than wrong. Falling back to global is
			// the only sane answer left.
			name:     "gateway with no location falls back to global",
			ctx:      base.PathContext{Project: "p", ResourceType: "gateways", ResourceName: "g1"},
			wantPath: "/projects/p/locations/global/gateways/g1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := networkServicesPathBuilder(tc.ctx); got != tc.wantPath {
				t.Errorf("got %q, want %q", got, tc.wantPath)
			}
		})
	}
}

// A region configured on the target must never reach a global collection's URL.
// ScopeGlobal already clears Location; this is the second lock on the same door,
// and the test is here so removing one is not silently fine.
func TestPathBuilderNoRegionLeakIntoGlobal(t *testing.T) {
	for _, collection := range []string{"meshes", "serviceLbPolicies"} {
		ctx := base.PathContext{
			Project:      "p",
			Region:       "europe-west1",
			Location:     "europe-west1",
			Zone:         "europe-west1-b",
			ResourceType: collection,
			ResourceName: "r1",
		}
		want := "/projects/p/locations/global/" + collection + "/r1"
		if got := networkServicesPathBuilder(ctx); got != want {
			t.Errorf("%s: region leaked: got %q, want %q", collection, got, want)
		}
	}
}

// On an async create the response is an Operation, not the resource, so the
// native id has to be built from context. The operation's own name must never
// be mistaken for it.
func TestNativeIDFromContext(t *testing.T) {
	ctx := base.PathContext{Project: "p", ResourceType: "meshes", ResourceName: "m1"}
	op := map[string]interface{}{
		"name": "projects/p/locations/global/operations/operation-123",
	}
	want := "projects/p/locations/global/meshes/m1"
	if got := extractNetworkServicesNativeID(op, ctx); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Discovery has no declared id to work from, so the id comes off the operation
// metadata or a direct resource read instead.
func TestNativeIDWithoutDeclaredName(t *testing.T) {
	ctx := base.PathContext{Project: "p", ResourceType: "gateways"}

	fromMetadata := map[string]interface{}{
		"name": "projects/p/locations/europe-central2/operations/op-1",
		"metadata": map[string]interface{}{
			"target": "projects/p/locations/europe-central2/gateways/g1",
		},
	}
	want := "projects/p/locations/europe-central2/gateways/g1"
	if got := extractNetworkServicesNativeID(fromMetadata, ctx); got != want {
		t.Errorf("metadata: got %q, want %q", got, want)
	}

	fromRead := map[string]interface{}{"name": want}
	if got := extractNetworkServicesNativeID(fromRead, ctx); got != want {
		t.Errorf("direct read: got %q, want %q", got, want)
	}
}

// Only an operation name is an operation id. A resource read must not be
// mistaken for one, or the plugin polls a path that is not an operation.
func TestOperationNameExtraction(t *testing.T) {
	op := map[string]interface{}{"name": "projects/p/locations/global/operations/op-1"}
	if got := extractOperationName(op); got != "projects/p/locations/global/operations/op-1" {
		t.Errorf("operation: got %q", got)
	}
	res := map[string]interface{}{"name": "projects/p/locations/global/meshes/m1"}
	if got := extractOperationName(res); got != "" {
		t.Errorf("resource read should yield no operation id, got %q", got)
	}
}

// This API accepts a create, hands back an operation, and can still fail it -
// the resource never appears and only the operation says why. A done operation
// carrying an error must surface as a failure, never as success.
func TestOperationStatusReportsFailure(t *testing.T) {
	for _, tc := range []struct {
		name    string
		op      map[string]interface{}
		done    bool
		wantErr string
	}{
		{
			name: "still running",
			op:   map[string]interface{}{"done": false},
			done: false,
		},
		{
			name: "done and clean",
			op:   map[string]interface{}{"done": true},
			done: true,
		},
		{
			name: "done but failed",
			op: map[string]interface{}{
				"done":  true,
				"error": map[string]interface{}{"message": "Config validation failed"},
			},
			done:    true,
			wantErr: "Config validation failed",
		},
		{
			name:    "failed with no message still fails",
			op:      map[string]interface{}{"done": true, "error": map[string]interface{}{}},
			done:    true,
			wantErr: "operation failed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			done, err := checkOperationStatus(tc.op)
			if done != tc.done {
				t.Errorf("done: got %v, want %v", done, tc.done)
			}
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("got %v, want %q", err, tc.wantErr)
			}
		})
	}
}
