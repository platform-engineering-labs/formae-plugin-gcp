// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package networkservices

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// autoCapacityDrain is a proto3 message whose only field is a bool, and the API
// omits that bool from its JSON when it is false. Send
//
//	"autoCapacityDrain": {"enable": false}
//
// and the read-back is
//
//	"autoCapacityDrain": {}
//
// so a forma that turns the drain off disagrees with state on every reconcile -
// declared {enable: false} against a stored {} - and the drift never settles.
//
// The field cannot be marked hasProviderDefault: schema hints are emitted for
// top-level fields only, and this one is nested inside a sub-resource. So the
// gap is closed here instead, by putting the omitted default back.
//
// `enable` is required in the schema (AutoCapacityDrain has no other field, so
// declaring the block without it says nothing), which is what makes this
// unambiguous: the block is present in a forma only when `enable` was declared,
// so injecting the false the API dropped always restores what was sent rather
// than inventing a value the user never wrote.
func serviceLbPolicyResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	out := base.ShortNameResponseTransformer.Transform(apiResponse, ctx)

	drain, ok := out["autoCapacityDrain"].(map[string]interface{})
	if !ok {
		// Absent entirely: the forma did not declare the block, and adding one
		// here would be the same drift in the other direction.
		return out
	}
	if _, present := drain["enable"]; present {
		return out
	}

	// Copy rather than mutate: the caller's map may be shared with the raw
	// response, and a transformer that writes through it is a surprise.
	restored := make(map[string]interface{}, len(drain)+1)
	for k, v := range drain {
		restored[k] = v
	}
	restored["enable"] = false
	out["autoCapacityDrain"] = restored
	return out
}
