// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package memcache implements GCP Cloud Memorystore for Memcached resources.
package memcache

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const InstanceResourceType = "GCP::Memcache::Instance"

var memcacheRegistry *base.ResourceRegistry

func init() {
	memcacheRegistry = base.NewResourceRegistry(
		MemcacheAPI, MemcacheOperations, MemcacheNativeID)

	// Update deferred (as artifactregistry/DNS/scheduler do) until PATCH is
	// verified live. create/delete are the async paths this batch proves out.
	err := memcacheRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: InstanceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "instances",
				Scope:         &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam: "instanceId", // id goes in ?instanceId=, not the body
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
	})
	if err != nil {
		panic(err)
	}
}
