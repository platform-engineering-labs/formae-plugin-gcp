// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package orgpolicy

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const PolicyResourceType = "GCP::OrgPolicy::Policy"

var orgPolicyRegistry *base.ResourceRegistry

func init() {
	orgPolicyRegistry = base.NewResourceRegistry(OrgPolicyAPI, OrgPolicyOperations, OrgPolicyNativeID)

	// Policy fits the generic engine: create is a plain POST to the collection
	// (/projects/{p}/policies) with the full-path "name" in the body; Read/Delete/
	// List operate on the full resource path. A RequestTransformer expands the
	// short constraint id to the full path the create body requires. Update is
	// deferred (SupportsUpdate:false) — the API's patch is a read-modify-write
	// with etag concurrency control that is not yet modeled.
	err := orgPolicyRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: PolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "policies",
				Scope:          &base.ScopeConfig{Type: base.ScopeProjectLevel},
				SupportsUpdate: false,
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			RequestTransformer:  base.RequestTransformerFunc(expandNameRequestTransformer),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
	})
	if err != nil {
		panic(err)
	}
}
