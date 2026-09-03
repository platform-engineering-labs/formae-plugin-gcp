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
	ServiceLbPolicyResourceType = "GCP::NetworkServices::ServiceLbPolicy"
	EndpointPolicyResourceType  = "GCP::NetworkServices::EndpointPolicy"

	HttpRouteResourceType = "GCP::NetworkServices::HttpRoute"
	GrpcRouteResourceType = "GCP::NetworkServices::GrpcRoute"
	TcpRouteResourceType  = "GCP::NetworkServices::TcpRoute"
	TlsRouteResourceType  = "GCP::NetworkServices::TlsRoute"
)

var networkServicesRegistry *base.ResourceRegistry

func init() {
	networkServicesRegistry = base.NewResourceRegistry(
		NetworkServicesAPI, NetworkServicesOperations, NetworkServicesNativeID)

	err := networkServicesRegistry.RegisterAll([]base.ResourceDefinition{
		{
			// The control-plane anchor of a Cloud Service Mesh: the scope that
			// routes attach to and that sidecars ask for their configuration.
			// A mesh is a name and a routing scope, nothing more - it allocates
			// no proxy and no address, and is free until a workload with an
			// Envoy sidecar joins it.
			ResourceType: MeshResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "meshes",
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "meshId", // id goes in ?meshId=, not the body
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			// Nothing but the name needs translating: every field is echoed
			// back exactly as sent. description, labels, interceptionPort and
			// envoyHeaders were each patched live and read back changed, so
			// none of them is create-only.
			RequestTransformer:  base.DropFieldsOnUpdate("name"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// How a global load balancer spreads traffic across backend
			// regions: the spray-or-waterfall algorithm, when to drain a
			// region automatically, the health floor that triggers failover,
			// and how strictly to keep traffic in its own region. A policy
			// enforces nothing on its own - a backend service names it - and
			// provisions nothing.
			ResourceType: ServiceLbPolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "serviceLbPolicies",
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "serviceLbPolicyId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			// The three nested config objects round-trip exactly, including
			// through a patch, so only the name is translated.
			RequestTransformer:  base.DropFieldsOnUpdate("name"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// Which configuration a mesh endpoint gets: match a set of
			// workloads by their metadata labels and hand them a TLS posture
			// and an authorization policy. This is the type that binds the mesh
			// to the Network Security policies the plugin already ships, which
			// is why it carries reference translation - see endpoint_policy.go.
			ResourceType: EndpointPolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "endpointPolicies",
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "endpointPolicyId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  base.RequestTransformerFunc(endpointPolicyRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(endpointPolicyResponseTransformer),
		},
		{
			// Layer-7 HTTP routing for a mesh: match on host, path, header or
			// query parameter, then redirect, rewrite, mirror, inject a fault
			// or forward to a backend service. Free as declared in the
			// conformance fixture, where the action is a redirect and so names
			// no backend at all.
			ResourceType: HttpRouteResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "httpRoutes",
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "httpRouteId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  base.RequestTransformerFunc(routeRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(routeResponseTransformer),
		},
		{
			// The same idea for gRPC: match on service and method rather than
			// path, and carry gRPC-shaped retry and fault-injection policies.
			// A rule needs an action but not a destination, so a
			// fault-injection action keeps the type free of a backend.
			ResourceType: GrpcRouteResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "grpcRoutes",
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "grpcRouteId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  base.RequestTransformerFunc(routeRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(routeResponseTransformer),
		},
		{
			// Layer-4 TCP routing: match a destination CIDR and port, then
			// forward to a backend service or hand the connection straight
			// through to its original destination. The passthrough action is
			// what keeps this free - but the API only allows it when a match
			// covers everything, so the fixture matches 0.0.0.0/0. See the
			// schema for the exact refusal.
			ResourceType: TcpRouteResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "tcpRoutes",
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "tcpRouteId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  base.RequestTransformerFunc(routeRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(routeResponseTransformer),
		},
		{
			// TLS routing by SNI and ALPN, without terminating the connection.
			// Alone among the four, this one has no destination-free action:
			// the API rejects a rule whose action names no destination, so the
			// fixture stands up a backend service - which is itself free with
			// no backends attached.
			ResourceType: TlsRouteResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "tlsRoutes",
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "tlsRouteId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  base.RequestTransformerFunc(routeRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(routeResponseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}
}
