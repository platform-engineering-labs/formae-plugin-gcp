// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package base

import (
	"reflect"
	"testing"
)

// The conformance harness's out-of-band path hands the plugin an evaluated forma
// with wrappers intact, so a secret arrives as {"$value": "..."} rather than a
// string and reaches the API as an empty field.
func TestUnwrapValuesUnwrapsOpaqueSecrets(t *testing.T) {
	got := UnwrapValues(map[string]interface{}{
		"name": "tunnel-a",
		"sharedSecret": map[string]interface{}{
			"$strategy": "Update", "$value": "psk", "$visibility": "Opaque",
		},
	})
	if got["sharedSecret"] != "psk" {
		t.Errorf("secret not unwrapped: %#v", got["sharedSecret"])
	}
	if got["name"] != "tunnel-a" {
		t.Errorf("plain field disturbed: %#v", got["name"])
	}
}

// A resolvable also carries "$"-prefixed keys but must survive untouched:
// formae resolves those, and a half-resolved reference must not be mistaken for
// a literal.
func TestUnwrapValuesLeavesResolvablesAlone(t *testing.T) {
	ref := map[string]interface{}{
		"$label": "router-a", "$property": "selfLink", "$res": true,
		"$type": "GCP::Compute::Router", "$value": "should-not-win",
	}
	got := UnwrapValues(map[string]interface{}{"router": ref})
	out, ok := got["router"].(map[string]interface{})
	if !ok || out["$res"] != true {
		t.Fatalf("resolvable was rewritten: %#v", got["router"])
	}
}

// Wrappers can sit anywhere: inside sub-objects and inside list elements.
func TestUnwrapValuesRecurses(t *testing.T) {
	got := UnwrapValues(map[string]interface{}{
		"nested": map[string]interface{}{
			"secret": map[string]interface{}{"$value": "inner", "$visibility": "Opaque"},
		},
		"list": []interface{}{
			map[string]interface{}{"k": map[string]interface{}{"$value": 42}},
			"plain",
		},
	})
	nested := got["nested"].(map[string]interface{})
	if nested["secret"] != "inner" {
		t.Errorf("nested wrapper survived: %#v", nested)
	}
	list := got["list"].([]interface{})
	if list[0].(map[string]interface{})["k"] != 42 || list[1] != "plain" {
		t.Errorf("list handling: %#v", list)
	}
}

// Ordinary properties must round-trip unchanged.
func TestUnwrapValuesLeavesPlainPropertiesAlone(t *testing.T) {
	in := map[string]interface{}{
		"name": "n", "count": float64(3), "flags": []interface{}{"a", "b"},
		"obj": map[string]interface{}{"x": "y"},
	}
	got := UnwrapValues(in)
	if !reflect.DeepEqual(got, in) {
		t.Errorf("plain properties changed: %#v", got)
	}
}
