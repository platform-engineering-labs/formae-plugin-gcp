// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package networkconnectivity implements GCP Network Connectivity resources.
package networkconnectivity

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const (
	HubResourceType                     = "GCP::NetworkConnectivity::Hub"
	InternalRangeResourceType           = "GCP::NetworkConnectivity::InternalRange"
	PolicyBasedRouteResourceType        = "GCP::NetworkConnectivity::PolicyBasedRoute"
	ServiceConnectionPolicyResourceType = "GCP::NetworkConnectivity::ServiceConnectionPolicy"
)

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
		{
			// A reservation of internal IP space in a VPC: the range is marked
			// as spoken for so nothing else is allocated over it.
			ResourceType: InternalRangeResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "internalRanges",
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "internalRangeId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			// name is the path, and the network and the range itself are fixed
			// at creation; leaving any of them in the body puts them in the
			// update mask, and the API rejects the patch.
			RequestTransformer:  base.DropFieldsOnUpdate("name", "network", "ipCidrRange", "prefixLength", "usage", "peering"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// A route matched on what the traffic is, not only where it is
			// going. Every field is fixed at creation - the API has a patch
			// method but rejects a change to any of them - so this one is
			// create/delete plus a replace.
			ResourceType: PolicyBasedRouteResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "policyBasedRoutes",
				Scope:         &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam: "policyBasedRouteId",
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			// See policy_based_route_network.go: this type is the only one here
			// that will not take a network self link, and reports back the form
			// it was given.
			RequestTransformer:  base.RequestTransformerFunc(policyBasedRouteRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(policyBasedRouteResponseTransformer),
		},
		{
			// Permission, in advance, for a managed service to place Private
			// Service Connect endpoints in a consumer's subnets.
			//
			// Regional, unlike everything else in this API. No ScopeGlobal
			// here: that clears ctx.Location, and this resource needs the
			// target's region in its URL.
			ResourceType: ServiceConnectionPolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "serviceConnectionPolicies",
				CreateIDParam:      "serviceConnectionPolicyId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			// The API names exactly what a patch may carry: description,
			// labels and psc_config. Anything else in the body enters the mask
			// and the patch is refused - "field \"network\" is not supported
			// in update_mask" - so the immutable trio is dropped on update.
			RequestTransformer:  base.DropFieldsOnUpdate("name", "serviceClass", "network"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
	})
	if err != nil {
		panic(err)
	}
}
