// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package datastream implements GCP Datastream resources.
package datastream

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const ConnectionProfileResourceType = "GCP::Datastream::ConnectionProfile"

var datastreamRegistry *base.ResourceRegistry

func init() {
	datastreamRegistry = base.NewResourceRegistry(
		DatastreamAPI, DatastreamOperations, DatastreamNativeID)

	// ponytail: Update deferred (as artifactregistry/dns/scheduler do) until
	// PATCH is verified live. create/delete are the async paths this batch
	// proves out.
	err := datastreamRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: ConnectionProfileResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "connectionProfiles",
				Scope:         &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam: "connectionProfileId", // id goes in ?connectionProfileId=, not the body
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
