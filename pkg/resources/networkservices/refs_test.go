// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package networkservices

import (
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestExpandGlobalRef(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		project    string
		collection string
		want       string
	}{
		{
			name:       "short name is expanded to a full path",
			value:      "my-mesh",
			project:    "proj-1",
			collection: "meshes",
			want:       "projects/proj-1/locations/global/meshes/my-mesh",
		},
		{
			name:       "a value that is already a path is left alone",
			value:      "projects/other/locations/global/meshes/shared",
			project:    "proj-1",
			collection: "meshes",
			want:       "projects/other/locations/global/meshes/shared",
		},
		{
			name:       "a full compute URL survives untouched",
			value:      "https://www.googleapis.com/compute/v1/projects/p/global/backendServices/bes",
			project:    "proj-1",
			collection: "meshes",
			want:       "https://www.googleapis.com/compute/v1/projects/p/global/backendServices/bes",
		},
		{
			name:       "empty stays empty rather than becoming a path to nothing",
			value:      "",
			project:    "proj-1",
			collection: "meshes",
			want:       "",
		},
		{
			name:       "the collection is not hard-coded to meshes",
			value:      "my-authz",
			project:    "proj-1",
			collection: "authorizationPolicies",
			want:       "projects/proj-1/locations/global/authorizationPolicies/my-authz",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expandGlobalRef(tt.value, tt.project, tt.collection); got != tt.want {
				t.Errorf("expandGlobalRef() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShortenRef(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "full path reduces to the short name",
			value: "projects/proj-1/locations/global/meshes/my-mesh",
			want:  "my-mesh",
		},
		{
			name:  "a bare short name is already short",
			value: "my-mesh",
			want:  "my-mesh",
		},
		{
			name:  "empty stays empty",
			value: "",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shortenRef(tt.value); got != tt.want {
				t.Errorf("shortenRef() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRefRoundTripIsIdentity is the test that matters. A reference is expanded
// on the way out and shortened on the way back, and if the two are not exact
// inverses the declared value and the stored value disagree forever: on an
// immutable field every re-apply plans a replacement that then fails.
func TestRefRoundTripIsIdentity(t *testing.T) {
	for _, collection := range []string{
		"meshes", "authorizationPolicies", "serverTlsPolicies", "clientTlsPolicies",
	} {
		for _, short := range []string{"my-thing", "formae-test-ns-mesh-abc123"} {
			expanded := expandGlobalRef(short, "proj-1", collection)
			if got := shortenRef(expanded); got != short {
				t.Errorf("round trip of %q via %q: got %q, want %q",
					short, collection, got, short)
			}
		}
	}
}

func TestExpandAndShortenRefList(t *testing.T) {
	in := []interface{}{"mesh-a", "projects/other/locations/global/meshes/mesh-b"}

	expanded := expandRefList(in, "proj-1", "meshes")
	want := []interface{}{
		"projects/proj-1/locations/global/meshes/mesh-a",
		"projects/other/locations/global/meshes/mesh-b",
	}
	if !reflect.DeepEqual(expanded, want) {
		t.Fatalf("expandRefList() = %#v, want %#v", expanded, want)
	}

	shortened := shortenRefList(expanded)
	wantShort := []interface{}{"mesh-a", "mesh-b"}
	if !reflect.DeepEqual(shortened, wantShort) {
		t.Errorf("shortenRefList() = %#v, want %#v", shortened, wantShort)
	}
}

// A non-list value must pass through rather than panic: the API has been seen
// to omit an unset listing entirely, and a malformed one must not crash a sync.
func TestRefListIgnoresNonLists(t *testing.T) {
	if got := expandRefList("not-a-list", "p", "meshes"); got != "not-a-list" {
		t.Errorf("expandRefList(non-list) = %#v, want passthrough", got)
	}
	if got := shortenRefList(42); got != 42 {
		t.Errorf("shortenRefList(non-list) = %#v, want passthrough", got)
	}
}

func TestRouteRequestTransformer(t *testing.T) {
	props := map[string]interface{}{
		"name":      "my-route",
		"hostnames": []interface{}{"a.example.com"},
		"meshes":    []interface{}{"my-mesh"},
	}

	// On create the name stays - it is not in the body's way, and the id also
	// travels as ?httpRouteId=.
	got, err := routeRequestTransformer(props, base.TransformContext{
		Project: "proj-1", Operation: resource.OperationCreate,
	})
	if err != nil {
		t.Fatalf("create: unexpected error: %v", err)
	}
	if got["name"] != "my-route" {
		t.Errorf("create dropped name: %#v", got["name"])
	}
	wantMeshes := []interface{}{"projects/proj-1/locations/global/meshes/my-mesh"}
	if !reflect.DeepEqual(got["meshes"], wantMeshes) {
		t.Errorf("create meshes = %#v, want %#v", got["meshes"], wantMeshes)
	}

	// On update the name must go: it is the path, and UpdateMaskFromBody would
	// otherwise put it in the update mask.
	got, err = routeRequestTransformer(props, base.TransformContext{
		Project: "proj-1", Operation: resource.OperationUpdate,
	})
	if err != nil {
		t.Fatalf("update: unexpected error: %v", err)
	}
	if _, present := got["name"]; present {
		t.Error("update kept name in the body")
	}
	// hostnames must survive an update: this API validates the whole resource
	// from the request body, so dropping a required field breaks the patch.
	if _, present := got["hostnames"]; !present {
		t.Error("update dropped hostnames, which the API requires on every patch")
	}

	// The input map must not be mutated - it is the caller's declared state.
	if !reflect.DeepEqual(props["meshes"], []interface{}{"my-mesh"}) {
		t.Errorf("transformer mutated its input: %#v", props["meshes"])
	}
}

func TestRouteResponseTransformer(t *testing.T) {
	resp := map[string]interface{}{
		"name":     "projects/proj-1/locations/global/httpRoutes/my-route",
		"meshes":   []interface{}{"projects/proj-1/locations/global/meshes/my-mesh"},
		"selfLink": "https://networkservices.googleapis.com/v1alpha1/projects/proj-1/locations/global/httpRoutes/my-route",
	}
	got := routeResponseTransformer(resp, base.TransformContext{Project: "proj-1"})

	if got["name"] != "my-route" {
		t.Errorf("name = %#v, want %q", got["name"], "my-route")
	}
	wantMeshes := []interface{}{"my-mesh"}
	if !reflect.DeepEqual(got["meshes"], wantMeshes) {
		t.Errorf("meshes = %#v, want %#v", got["meshes"], wantMeshes)
	}
}

func TestEndpointPolicyRequestTransformer(t *testing.T) {
	props := map[string]interface{}{
		"name":                "my-ep",
		"type":                "SIDECAR_PROXY",
		"endpointMatcher":     map[string]interface{}{"metadataLabelMatcher": map[string]interface{}{}},
		"authorizationPolicy": "my-authz",
		"serverTlsPolicy":     "my-server-tls",
		"clientTlsPolicy":     "projects/other/locations/global/clientTlsPolicies/shared",
	}

	got, err := endpointPolicyRequestTransformer(props, base.TransformContext{
		Project: "proj-1", Operation: resource.OperationUpdate,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, present := got["name"]; present {
		t.Error("update kept name in the body")
	}
	// type and endpointMatcher must ride along on every patch or the API
	// refuses the call outright.
	if got["type"] != "SIDECAR_PROXY" {
		t.Error("update dropped type, which the API requires on every patch")
	}
	if _, present := got["endpointMatcher"]; !present {
		t.Error("update dropped endpointMatcher, which the API requires on every patch")
	}

	if got["authorizationPolicy"] != "projects/proj-1/locations/global/authorizationPolicies/my-authz" {
		t.Errorf("authorizationPolicy = %#v", got["authorizationPolicy"])
	}
	if got["serverTlsPolicy"] != "projects/proj-1/locations/global/serverTlsPolicies/my-server-tls" {
		t.Errorf("serverTlsPolicy = %#v", got["serverTlsPolicy"])
	}
	if got["clientTlsPolicy"] != "projects/other/locations/global/clientTlsPolicies/shared" {
		t.Errorf("clientTlsPolicy should have passed through: %#v", got["clientTlsPolicy"])
	}
}

func TestEndpointPolicyResponseTransformer(t *testing.T) {
	resp := map[string]interface{}{
		"name":                "projects/proj-1/locations/global/endpointPolicies/my-ep",
		"authorizationPolicy": "projects/proj-1/locations/global/authorizationPolicies/my-authz",
		"serverTlsPolicy":     "projects/proj-1/locations/global/serverTlsPolicies/my-server-tls",
	}
	got := endpointPolicyResponseTransformer(resp, base.TransformContext{Project: "proj-1"})

	if got["name"] != "my-ep" {
		t.Errorf("name = %#v", got["name"])
	}
	if got["authorizationPolicy"] != "my-authz" {
		t.Errorf("authorizationPolicy = %#v", got["authorizationPolicy"])
	}
	if got["serverTlsPolicy"] != "my-server-tls" {
		t.Errorf("serverTlsPolicy = %#v", got["serverTlsPolicy"])
	}
}

// TestEndpointPolicyRefRoundTrip pins the same identity as the mesh case, for
// each of the three collections an endpoint policy can name.
func TestEndpointPolicyRefRoundTrip(t *testing.T) {
	for field, collection := range endpointPolicyRefCollections {
		short := "formae-test-ns-ep-target"
		expanded := expandGlobalRef(short, "proj-1", collection)
		if got := shortenRef(expanded); got != short {
			t.Errorf("%s round trip: got %q, want %q", field, got, short)
		}
	}
}
