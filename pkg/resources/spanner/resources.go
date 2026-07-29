// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package spanner implements GCP Cloud Spanner resources.
package spanner

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const InstanceResourceType = "GCP::Spanner::Instance"

var spannerRegistry *base.ResourceRegistry

// spannerInstanceBodyBuilder converts the flat declared properties into the
// instances.create body shape:
//
//	{"instanceId": "<id>", "instance": {config, displayName, nodeCount|processingUnits, labels}}
//
// The id is a sibling BODY field ("instanceId") and the resource is wrapped
// under "instance", so a plain RequestWrapper can't add the sibling id — this
// transformer emits both. "name" is lifted out to "instanceId" and dropped from
// the inner object (the API rejects/ignores a name on the wrapped instance).
func spannerInstanceBodyBuilder(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
	name, _ := props["name"].(string)

	instance := make(map[string]interface{}, len(props))
	for k, v := range props {
		if k == "name" {
			continue
		}
		instance[k] = v
	}

	return map[string]interface{}{
		"instanceId": name,
		"instance":   instance,
	}, nil
}

func init() {
	spannerRegistry = base.NewResourceRegistry(SpannerAPI, SpannerOperations, SpannerNativeID)

	// Update deferred (SupportsUpdate:false) — conformance skips it. create is
	// the async LRO path; delete returns Empty and base.Delete treats an empty
	// operation id as immediate success.
	err := spannerRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: InstanceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "instances",
				Scope:          &base.ScopeConfig{Type: base.ScopeProjectLevel},
				SupportsUpdate: false,
			},
			RequestTransformer:  base.RequestTransformerFunc(spannerInstanceBodyBuilder),
			ResponseTransformer: base.ShortNameResponseTransformer,
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
		},
	})
	if err != nil {
		panic(err)
	}
}
