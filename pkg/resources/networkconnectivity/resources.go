// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package networkconnectivity implements GCP Network Connectivity resources.
package networkconnectivity

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const HubResourceType = "GCP::NetworkConnectivity::Hub"

var networkConnectivityRegistry *base.ResourceRegistry

func init() {
	networkConnectivityRegistry = base.NewResourceRegistry(
		NetworkConnectivityAPI, NetworkConnectivityOperations, NetworkConnectivityNativeID)

	// ponytail: Update deferred (as ArtifactRegistry/DNS do) until PATCH is
	// verified live. create/delete are the async paths this batch proves out.
	err := networkConnectivityRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: HubResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "hubs",
				Scope:         &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam: "hubId", // id goes in ?hubId=, not the body
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
