// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package parametermanager

import (
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const (
	versionPath   = "projects/p/locations/global/parameters/param/versions/v1"
	parameterPath = "projects/p/locations/global/parameters/param"
)

// The declared `data` string is base64 encoded into the nested payload the API
// requires, and `parameter` never reaches the body.
func TestParameterVersionRequestTransformerCreate(t *testing.T) {
	got, err := parameterVersionRequestTransformer.Transform(
		map[string]interface{}{
			"name":      "v1",
			"parameter": "param",
			"data":      "hello",
			"disabled":  false,
		},
		base.TransformContext{Operation: resource.OperationCreate},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]interface{}{
		"name":     "v1",
		"disabled": false,
		"payload":  map[string]interface{}{"data": "aGVsbG8="},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
}

// An update body may carry nothing but the mutable field. Anything else lands
// in the updateMask, which is built from the body's top-level keys, and the
// API answers 400 IMMUTABLE_FIELD for name and for payload alike.
func TestParameterVersionRequestTransformerUpdate(t *testing.T) {
	got, err := parameterVersionRequestTransformer.Transform(
		map[string]interface{}{
			"name":      "v1",
			"parameter": "param",
			"data":      "hello",
			"disabled":  true,
		},
		base.TransformContext{Operation: resource.OperationUpdate},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]interface{}{"disabled": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
}

// The payload never survives a response. This is the security assertion: a
// version's contents are user data and must not reach stored state, and the API
// returns them on both create and read.
func TestParameterVersionResponseTransformer(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]interface{}
		want map[string]interface{}
	}{
		{
			name: "payload is stripped and the parent recovered from the path",
			in: map[string]interface{}{
				"name":    versionPath,
				"payload": map[string]interface{}{"data": "aGVsbG8="},
			},
			want: map[string]interface{}{
				"name":      "v1",
				"parameter": "param",
			},
		},
		{
			name: "a disabled version keeps its flag",
			in: map[string]interface{}{
				"name":     versionPath,
				"disabled": true,
			},
			want: map[string]interface{}{
				"name":      "v1",
				"parameter": "param",
				"disabled":  true,
			},
		},
		{
			name: "a response that is not a version gains no parent",
			in: map[string]interface{}{
				"name": parameterPath,
			},
			want: map[string]interface{}{
				"name": "param",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parameterVersionResponseTransformer.Transform(tt.in, base.TransformContext{})
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %#v\nwant %#v", got, tt.want)
			}
		})
	}
}

// The round trip a forma sees has to be the identity, or an immutable field
// disagrees with stored state forever and every re-apply plans a replace that
// then fails. `parameter` is declared as the short id, expanded into the path by
// the URL builder and lifted back out of the reported path by the response
// transformer; `name` is declared short and reported long.
func TestParameterVersionIdentityRoundTrip(t *testing.T) {
	declared := map[string]interface{}{
		"name":      "v1",
		"parameter": "param",
	}

	body, err := parameterVersionRequestTransformer.Transform(
		map[string]interface{}{
			"name":      "v1",
			"parameter": "param",
			"data":      "hello",
		},
		base.TransformContext{Operation: resource.OperationCreate},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// What the API answers a create with: the resource, at its full path, with
	// the payload echoed back.
	apiResponse := map[string]interface{}{
		"name":    versionPath,
		"payload": body["payload"],
	}

	got := parameterVersionResponseTransformer.Transform(apiResponse, base.TransformContext{})
	if !reflect.DeepEqual(got, declared) {
		t.Errorf("round trip is not the identity:\ngot  %#v\nwant %#v", got, declared)
	}
}
