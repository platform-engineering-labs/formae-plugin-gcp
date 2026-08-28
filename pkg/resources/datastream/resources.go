// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package datastream implements GCP Datastream resources.
package datastream

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const (
	ConnectionProfileResourceType = "GCP::Datastream::ConnectionProfile"
	StreamResourceType            = "GCP::Datastream::Stream"
	PrivateConnectionResourceType = "GCP::Datastream::PrivateConnection"
	RouteResourceType             = "GCP::Datastream::Route"
)

func readOnlyLifecycleOps() []resource.Operation {
	return []resource.Operation{
		resource.OperationCreate,
		resource.OperationRead,
		resource.OperationDelete,
		resource.OperationList,
		resource.OperationCheckStatus,
	}
}

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
		{
			// What actually moves data: a source connection profile, a
			// destination connection profile, and the config joining them. A
			// connection profile on its own moves nothing.
			ResourceType: StreamResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "streams",
				Scope:         &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam: "streamId",
			},
			Operations:          readOnlyLifecycleOps(),
			RequestTransformer:  base.RequestTransformerFunc(streamRequest),
			ResponseTransformer: base.ResponseTransformerFunc(streamResponse),
		},
		{
			// Private connectivity to a source that is not reachable over the
			// public internet: a VPC peering between the project's network and
			// Datastream's.
			ResourceType: PrivateConnectionResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "privateConnections",
				Scope:         &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam: "privateConnectionId",
			},
			Operations:          readOnlyLifecycleOps(),
			RequestTransformer:  base.RequestTransformerFunc(dropDatastreamPathFields),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// A route tells a private connection which address to reach the
			// source on. It only exists inside one.
			ResourceType: RouteResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "routes",
				Scope:        &base.ScopeConfig{Type: base.ScopeLocationBased},
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "privateConnections",
					PropertyName:   "privateConnection",
					RequiresParent: true,
				},
				CreateIDParam: "routeId",
			},
			Operations:          readOnlyLifecycleOps(),
			RequestTransformer:  base.RequestTransformerFunc(dropDatastreamPathFields),
			ResponseTransformer: base.ResponseTransformerFunc(routeResponse),
		},
	})
	if err != nil {
		panic(err)
	}
}
