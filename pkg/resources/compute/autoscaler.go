// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// autoscalerAPITargetField is the Compute API's name for the managed instance
// group an autoscaler scales.
const autoscalerAPITargetField = "target"

// autoscalerSchemaTargetField is what the schema calls it. "target" is reserved
// by formae's base Resource class (it names the deployment target), so a schema
// field of that name is swallowed by the forma renderer before the plugin ever
// sees it.
const autoscalerSchemaTargetField = "instanceGroupManager"

// autoscalerRequestTransformer renames instanceGroupManager -> target.
var autoscalerRequestTransformer = base.RequestTransformerFunc(
	func(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
		return renameKey(props, autoscalerSchemaTargetField, autoscalerAPITargetField), nil
	})

// autoscalerResponseTransformer renames target -> instanceGroupManager so the
// stored state round-trips against the declared forma, and shortens the zone URL.
var autoscalerResponseTransformer = &base.CompositeResponseTransformer{
	Transformers: []base.ResponseTransformer{
		base.ZoneResponseTransformer,
		base.ResponseTransformerFunc(
			func(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
				return renameKey(apiResponse, autoscalerAPITargetField, autoscalerSchemaTargetField)
			}),
	},
}

// regionAutoscalerResponseTransformer is the regional twin: the API reports
// "region" as a full URL instead of "zone", and the same target rename applies.
var regionAutoscalerResponseTransformer = &base.CompositeResponseTransformer{
	Transformers: []base.ResponseTransformer{
		base.RegionResponseTransformer,
		base.ResponseTransformerFunc(
			func(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
				return renameKey(apiResponse, autoscalerAPITargetField, autoscalerSchemaTargetField)
			}),
	},
}

// renameKey copies props with `from` re-keyed to `to`. Absent or empty `from`
// leaves the map untouched, so a partial read never invents a field.
func renameKey(props map[string]interface{}, from, to string) map[string]interface{} {
	val, ok := props[from]
	if !ok {
		return props
	}
	result := make(map[string]interface{}, len(props))
	for k, v := range props {
		if k == from {
			continue
		}
		result[k] = v
	}
	result[to] = val
	return result
}
