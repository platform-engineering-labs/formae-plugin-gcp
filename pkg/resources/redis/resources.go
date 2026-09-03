// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package redis implements GCP Memorystore for Redis resources.
package redis

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const (
	InstanceResourceType  = "GCP::Redis::Instance"
	AclPolicyResourceType = "GCP::Redis::AclPolicy"
)

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
		{
			// A standalone ACL configuration object for Memorystore for Redis
			// Cluster: a named set of Redis OSS ACL rules that clusters attach.
			// Creating one provisions nothing and costs nothing - it is billed
			// only through the clusters that reference it, and this plugin
			// deliberately creates no cluster here.
			//
			// The one type in this API that is NOT an LRO: create returns the
			// resource, not an Operation. RedisSyncOperations overrides the
			// registry-wide async config for this definition only - see the
			// comment there for why it must still set OperationIDExtractor.
			ResourceType:    AclPolicyResourceType,
			OperationConfig: RedisSyncOperations,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "aclPolicies",
				Scope:         &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam: "aclPolicyId", // id goes in ?aclPolicyId=, not the body
				// PATCH v1/{+name} takes an AclPolicy body and an updateMask
				// query parameter; `rules` is the only mutable field, and the
				// request transformer removes everything else so the computed
				// mask is exactly that.
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
				// The delete is accepted immediately but the policy keeps
				// reading back as "state": "DELETING" for another fifteen to
				// twenty seconds; see aclPolicyDeleting.
				ReadTreatAsMissing: aclPolicyDeleting,
			},
			RequestTransformer:  base.RequestTransformerFunc(aclPolicyRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(aclPolicyResponseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}
}
