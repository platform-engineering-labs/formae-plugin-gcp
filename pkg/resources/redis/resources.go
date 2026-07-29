// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package redis implements GCP Memorystore for Redis resources.
package redis

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const InstanceResourceType = "GCP::Redis::Instance"

var redisRegistry *base.ResourceRegistry

func init() {
	redisRegistry = base.NewResourceRegistry(RedisAPI, RedisOperations, RedisNativeID)

	// ponytail: Update deferred (as artifactregistry/DNS do) until PATCH is
	// verified live. create/delete are the async paths this batch proves out.
	err := redisRegistry.RegisterAll([]base.ResourceDefinition{
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
