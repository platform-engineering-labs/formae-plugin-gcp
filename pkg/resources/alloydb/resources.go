// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package alloydb implements GCP AlloyDB resources.
package alloydb

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const ClusterResourceType = "GCP::AlloyDB::Cluster"

var alloyDBRegistry *base.ResourceRegistry

func init() {
	alloyDBRegistry = base.NewResourceRegistry(
		AlloyDBAPI, AlloyDBOperations, AlloyDBNativeID)

	// ponytail: Update deferred (as DNS/CloudRun/artifactregistry do) until
	// PATCH is verified live. create/delete are the async paths this proves out.
	err := alloyDBRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: ClusterResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "clusters",
				Scope:         &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam: "clusterId", // id goes in ?clusterId=, not the body
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
