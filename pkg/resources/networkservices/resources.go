// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package networkservices implements GCP Network Services resources.
package networkservices

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

const (
	MeshResourceType            = "GCP::NetworkServices::Mesh"
	GatewayResourceType         = "GCP::NetworkServices::Gateway"
	ServiceLbPolicyResourceType = "GCP::NetworkServices::ServiceLbPolicy"
)

var networkServicesRegistry *base.ResourceRegistry

func init() {
	networkServicesRegistry = base.NewResourceRegistry(
		NetworkServicesAPI, NetworkServicesOperations, NetworkServicesNativeID)

	// No explicit Operations list on any definition below: base.StandardOperations
	// (create/read/update/delete/list/checkStatus) is exactly right for all
	// three. Unlike networkconnectivity, where Update was deferred until PATCH
	// could be verified, PATCH was exercised live against each of these types -
	// including which fields the update mask will actually accept - before this
	// landed.
	err := networkServicesRegistry.RegisterAll([]base.ResourceDefinition{
		{
			// The config root of a service mesh: the sidecars that share a mesh
			// are handed one routing table, assembled from the routes that name
			// it. The mesh itself carries almost nothing - it is the thing
			// routes point at.
			ResourceType: MeshResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "meshes",
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "meshId", // id goes in ?meshId=, not the body
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			// name is the path. Everything else a mesh has - description,
			// labels, interceptionPort, envoyHeaders - is patchable, so nothing
			// else is dropped.
			RequestTransformer:  base.DropFieldsOnUpdate("name"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// The ingress half of the same mesh: where traffic from outside
			// enters. Regional, unlike the rest of this batch, so no
			// ScopeGlobal here - that clears ctx.Location, and a gateway needs
			// the target's region in its URL.
			ResourceType: GatewayResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "gateways",
				CreateIDParam:      "gatewayId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			// Two fields must leave the update body, for two different reasons.
			//
			// "type" is refused outright - "Gateway type can not be updated
			// once created" - so leaving it in the body puts it in the mask and
			// the whole patch fails.
			//
			// "scope" is worse: the API accepts it in the mask, reports the
			// operation as successfully done, and silently keeps the old value.
			// Left in, a changed scope would look applied and never be, so the
			// forma and state would disagree forever with nothing to show for
			// it. Both are createOnly in the schema, so a change to either
			// plans a replacement instead - which is the only way to actually
			// move them.
			//
			// Dropping them is safe: a gateway PATCH preserves a scalar field
			// that is absent from the body (only labels are cleared when
			// omitted, and formae always sends the full declared body).
			RequestTransformer:  base.DropFieldsOnUpdate("name", "type", "scope"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// How a backend service spreads traffic across regions, and what it
			// does when a region gets unhealthy. Attached to a backend service
			// by that service, not from here.
			ResourceType: ServiceLbPolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "serviceLbPolicies",
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "serviceLbPolicyId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer: base.DropFieldsOnUpdate("name"),
			// See service_lb_policy_capacity_drain.go: the API drops a false
			// "enable" out of autoCapacityDrain on the way back.
			ResponseTransformer: base.ResponseTransformerFunc(serviceLbPolicyResponseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}
}
