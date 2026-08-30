// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// Resource type constants
const (
	AddressResourceType               = "GCP::Compute::Address"
	DiskResourceType                  = "GCP::Compute::Disk"
	FirewallResourceType              = "GCP::Compute::Firewall"
	NetworkFirewallPolicyResourceType = "GCP::Compute::NetworkFirewallPolicy"
	// Registered in firewall_policy_rule.go: every operation is a verb on the policy.
	NetworkFirewallPolicyRuleResourceType = "GCP::Compute::NetworkFirewallPolicyRule"
	// Registered in firewall_policy_association.go: add/get/removeAssociation verbs.
	NetworkFirewallPolicyAssociationResourceType       = "GCP::Compute::NetworkFirewallPolicyAssociation"
	RegionNetworkFirewallPolicyAssociationResourceType = "GCP::Compute::RegionNetworkFirewallPolicyAssociation"
	RegionNetworkFirewallPolicyResourceType            = "GCP::Compute::RegionNetworkFirewallPolicy"
	InstanceResourceType                               = "GCP::Compute::Instance"
	NetworkResourceType                                = "GCP::Compute::Network"
	// Registered in network_peering.go: addPeering/removePeering verbs.
	NetworkPeeringResourceType = "GCP::Compute::NetworkPeering"
	RouterResourceType         = "GCP::Compute::Router"
	RouterNatResourceType      = "GCP::Compute::RouterNat"
	// Registered in router_interface.go: merge into Router.interfaces[].
	RouterInterfaceResourceType = "GCP::Compute::RouterInterface"
	// Registered in router_route_policy.go: update/get/delete/listRoutePolicies verbs.
	RouterRoutePolicyResourceType = "GCP::Compute::RouterRoutePolicy"
	RouterNamedSetResourceType    = "GCP::Compute::RouterNamedSet"
	SubnetworkResourceType        = "GCP::Compute::Subnetwork"
	RouteResourceType             = "GCP::Compute::Route"
	// Registered in project_metadata_item.go: setCommonInstanceMetadata merge.
	ProjectMetadataItemResourceType  = "GCP::Compute::ProjectMetadataItem"
	SecurityPolicyResourceType       = "GCP::Compute::SecurityPolicy"
	RegionSecurityPolicyResourceType = "GCP::Compute::RegionSecurityPolicy"
	// Registered in policy_rule.go: add/get/patch/removeRule verbs.
	SecurityPolicyRuleResourceType       = "GCP::Compute::SecurityPolicyRule"
	RegionSecurityPolicyRuleResourceType = "GCP::Compute::RegionSecurityPolicyRule"

	// Managed instance groups
	InstanceTemplateResourceType           = "GCP::Compute::InstanceTemplate"
	RegionInstanceTemplateResourceType     = "GCP::Compute::RegionInstanceTemplate"
	InstanceGroupManagerResourceType       = "GCP::Compute::InstanceGroupManager"
	RegionInstanceGroupManagerResourceType = "GCP::Compute::RegionInstanceGroupManager"
	AutoscalerResourceType                 = "GCP::Compute::Autoscaler"
	RegionAutoscalerResourceType           = "GCP::Compute::RegionAutoscaler"

	// Load Balancer backends + certificate
	InstanceGroupResourceType              = "GCP::Compute::InstanceGroup"
	NetworkEndpointGroupResourceType       = "GCP::Compute::NetworkEndpointGroup"
	GlobalNetworkEndpointGroupResourceType = "GCP::Compute::GlobalNetworkEndpointGroup"
	// Registered in global_network_endpoint.go: attach/detachNetworkEndpoints verbs.
	GlobalNetworkEndpointResourceType = "GCP::Compute::GlobalNetworkEndpoint"
	BackendBucketResourceType         = "GCP::Compute::BackendBucket"
	ResourcePolicyResourceType        = "GCP::Compute::ResourcePolicy"
	// Registered in disk_resource_policy_attachment.go: add/removeResourcePolicies verbs.
	DiskResourcePolicyAttachmentResourceType = "GCP::Compute::DiskResourcePolicyAttachment"
	// Registered in disk_async_replication.go: start/stopAsyncReplication verbs.
	DiskAsyncReplicationResourceType               = "GCP::Compute::DiskAsyncReplication"
	RegionDiskResourcePolicyAttachmentResourceType = "GCP::Compute::RegionDiskResourcePolicyAttachment"
	ImageResourceType                              = "GCP::Compute::Image"
	MachineImageResourceType                       = "GCP::Compute::MachineImage"
	SnapshotResourceType                           = "GCP::Compute::Snapshot"
	InstantSnapshotResourceType                    = "GCP::Compute::InstantSnapshot"
	RegionInstantSnapshotResourceType              = "GCP::Compute::RegionInstantSnapshot"
	RegionSnapshotResourceType                     = "GCP::Compute::RegionSnapshot"
	NodeTemplateResourceType                       = "GCP::Compute::NodeTemplate"
	RegionDiskResourceType                         = "GCP::Compute::RegionDisk"

	// HA VPN
	ExternalVpnGatewayResourceType         = "GCP::Compute::ExternalVpnGateway"
	HaVpnGatewayResourceType               = "GCP::Compute::HaVpnGateway"
	TargetVpnGatewayResourceType           = "GCP::Compute::TargetVpnGateway"
	VpnTunnelResourceType                  = "GCP::Compute::VpnTunnel"
	SslCertificateResourceType             = "GCP::Compute::SslCertificate"
	SslPolicyResourceType                  = "GCP::Compute::SslPolicy"
	RegionSslPolicyResourceType            = "GCP::Compute::RegionSslPolicy"
	RegionNotificationEndpointResourceType = "GCP::Compute::RegionNotificationEndpoint"
	HttpHealthCheckResourceType            = "GCP::Compute::HttpHealthCheck"
	HttpsHealthCheckResourceType           = "GCP::Compute::HttpsHealthCheck"
	NetworkAttachmentResourceType          = "GCP::Compute::NetworkAttachment"
	ServiceAttachmentResourceType          = "GCP::Compute::ServiceAttachment"

	// Load Balancer - Global resources
	GlobalAddressResourceType  = "GCP::Compute::GlobalAddress"
	HealthCheckResourceType    = "GCP::Compute::HealthCheck"
	BackendServiceResourceType = "GCP::Compute::BackendService"
	// Registered in backend_service_signed_url_key.go: add/deleteSignedUrlKey verbs.
	BackendServiceSignedUrlKeyResourceType = "GCP::Compute::BackendServiceSignedUrlKey"
	UrlMapResourceType                     = "GCP::Compute::UrlMap"
	TargetHttpProxyResourceType            = "GCP::Compute::TargetHttpProxy"
	TargetHttpsProxyResourceType           = "GCP::Compute::TargetHttpsProxy"
	TargetTcpProxyResourceType             = "GCP::Compute::TargetTcpProxy"
	TargetSslProxyResourceType             = "GCP::Compute::TargetSslProxy"
	TargetGrpcProxyResourceType            = "GCP::Compute::TargetGrpcProxy"
	GlobalForwardingRuleResourceType       = "GCP::Compute::GlobalForwardingRule"

	// Load Balancer - Regional resources
	RegionHealthCheckResourceType             = "GCP::Compute::RegionHealthCheck"
	RegionHealthAggregationPolicyResourceType = "GCP::Compute::RegionHealthAggregationPolicy"
	RegionHealthSourceResourceType            = "GCP::Compute::RegionHealthSource"
	RegionCompositeHealthCheckResourceType    = "GCP::Compute::RegionCompositeHealthCheck"
	RegionBackendServiceResourceType          = "GCP::Compute::RegionBackendService"
	RegionUrlMapResourceType                  = "GCP::Compute::RegionUrlMap"
	RegionTargetHttpProxyResourceType         = "GCP::Compute::RegionTargetHttpProxy"
	RegionTargetHttpsProxyResourceType        = "GCP::Compute::RegionTargetHttpsProxy"
	RegionTargetTcpProxyResourceType          = "GCP::Compute::RegionTargetTcpProxy"
	ForwardingRuleResourceType                = "GCP::Compute::ForwardingRule"
	TargetPoolResourceType                    = "GCP::Compute::TargetPool"
	TargetInstanceResourceType                = "GCP::Compute::TargetInstance"
)

// computeRegistry is the unified registry for all Compute resources
var computeRegistry *base.ResourceRegistry

func NewComputeProvisioner(cfg *config.Config, resourceType string) (prov.Provisioner, error) {
	if resourceType == RouterNatResourceType {
		return NewRouterNatProvisioner(cfg), nil
	}

	if resourceType == InstanceGroupResourceType {
		return newInstanceGroupProvisioner(cfg), nil
	}

	if computeRegistry == nil {
		return nil, fmt.Errorf("compute registry not initialized")
	}

	_, ok := computeRegistry.Definitions[resourceType]
	if !ok {
		return nil, fmt.Errorf("no configuration found for resource type: %s", resourceType)
	}

	// Use the registry's provisioner creation
	return computeRegistry.CreateProvisioner(cfg, resourceType), nil
}

// Wrapper functions to adapt transformers to base interfaces
func wrapInstanceBodyBuilder(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	return instanceBodyBuilder(props, ctx)
}

func wrapInstanceResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	return instanceResponseTransformer(apiResponse, ctx)
}

func init() {
	// Create the registry with common Compute API configurations
	computeRegistry = base.NewResourceRegistry(
		ComputeAPI,
		ComputeOperations,
		ComputeNativeID,
	)

	// Register all Compute resources
	err := computeRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: AddressResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "addresses",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    false, // Addresses are immutable
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil, // Pass through properties
			ResponseTransformer: base.RegionResponseTransformer,
		},
		// Note: DiskResourceType is registered separately in disk.go with custom setLabels update handler
		{
			ResourceType: InstanceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "instances",
				Scope: &base.ScopeConfig{
					Type: base.ScopeZonal,
				},
				SupportsUpdate: true,
				// Use PUT for full resource replacement - required by GCP Compute Engine API
				// The instances.update method doesn't support PATCH semantics; it requires
				// the complete instance resource with all properties.
				UpdateMethod: base.UpdateMethodPut,
				// mostDisruptiveAllowedAction=REFRESH allows label/metadata updates without restart
				UpdateQueryParams: map[string]string{
					"mostDisruptiveAllowedAction": "REFRESH",
				},
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       true,
					FieldName:     "labelFingerprint",
					LocationInURL: false,
				},
			},
			RequestTransformer:  base.RequestTransformerFunc(wrapInstanceBodyBuilder),
			ResponseTransformer: base.ResponseTransformerFunc(wrapInstanceResponseTransformer),
		},
		{
			ResourceType: NetworkResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "networks",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate:    true,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: NetworkResponseTransformer,
		},
		{
			ResourceType: FirewallResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "firewalls",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate: true,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       true,
					FieldName:     "fingerprint",
					LocationInURL: false,
				},
			},
			RequestTransformer:  nil,
			ResponseTransformer: FirewallResponseTransformer,
		},
		{
			ResourceType: SubnetworkResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "subnetworks",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate: true,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       true,
					FieldName:     "fingerprint",
					LocationInURL: false,
				},
			},
			RequestTransformer:  nil,
			ResponseTransformer: SubnetworkResponseTransformer,
		},
		{
			ResourceType: RouterResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "routers",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    true,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: RouterResponseTransformer,
		},

		// ==================== Load Balancer - Backends + Certificate ====================

		// SSL Certificate - Global managed/self-managed cert for HTTPS proxies.
		// Immutable (create + delete only).
		{
			ResourceType: SslCertificateResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "sslCertificates",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate:    false, // Certificates are immutable
				OptimisticLocking: nil,
			},
			RequestTransformer:  base.RequestTransformerFunc(sslCertificateRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(sslCertificateResponseTransformer),
		},

		// Global Network Endpoint Group - internet NEG, so an external LB or CDN
		// can serve from an origin outside Google Cloud. Endpoints are attached
		// with attachNetworkEndpoints and are not modelled. ponytail: no patch
		// endpoint exists, so a change replaces.
		{
			ResourceType: GlobalNetworkEndpointGroupResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "networkEndpointGroups",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// Network Endpoint Group - Regional serverless NEG (Cloud Run backend).
		// Immutable (create + delete only).
		{
			ResourceType: NetworkEndpointGroupResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "networkEndpointGroups",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    false, // NEGs are immutable
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		// Note: InstanceGroupResourceType is registered separately in
		// instance_group.go with a custom provisioner (membership reconcile via
		// addInstances/removeInstances + namedPorts via setNamedPorts).

		// ==================== Load Balancer - Global Resources ====================

		// Global Address - For external/internal global static IPs
		{
			ResourceType: GlobalAddressResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "addresses",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate:    false, // Addresses are immutable
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// Route - Global static custom routes within a VPC network
		{
			ResourceType: RouteResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "routes",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate:    false, // Routes are immutable
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// Security Policy - Cloud Armor global security policy
		// Service Attachment - regional PSC producer endpoint: publishes an
		// internal LB to consumer VPCs. patch requires the current fingerprint,
		// same as networkAttachments.
		{
			ResourceType: ServiceAttachmentResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "serviceAttachments",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPatch,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       true,
					FieldName:     "fingerprint",
					LocationInURL: false,
				},
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		// Network Attachment - regional PSC-interface consumer endpoint.
		// networkAttachments.patch rejects a body without the current
		// fingerprint ("Required field 'resource.fingerprint' not specified"),
		// so updates go through optimistic locking.
		{
			ResourceType: NetworkAttachmentResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "networkAttachments",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPatch,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       true,
					FieldName:     "fingerprint",
					LocationInURL: false,
				},
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		// Target Instance - zonal protocol-forwarding target: one VM behind a
		// forwarding rule. ponytail: no patch endpoint, so a change replaces.
		{
			ResourceType: TargetInstanceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "targetInstances",
				Scope: &base.ScopeConfig{
					Type: base.ScopeZonal,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.ZoneResponseTransformer,
		},

		// Region Health Source - binds a backend service to a health aggregation
		// policy. NOTE the URL segment is "healthSources" while the method group
		// is "regionHealthSources". ponytail: patch needs the fingerprint dance;
		// deferred, so a change replaces.
		// Region Composite Health Check - aggregates health sources and reports the
		// verdict at a forwarding rule, which is what makes a health source and
		// its aggregation policy do anything.
		//
		// Patch needs the fingerprint, and the API hides that: a patch without
		// one is accepted with a 200 and an operation, then the *operation*
		// fails with 412 CONDITION_NOT_MET ("missing fingerprint"). Only the
		// operation outcome tells the truth.
		{
			ResourceType: RegionCompositeHealthCheckResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "compositeHealthChecks",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate: true,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       true,
					FieldName:     "fingerprint",
					LocationInURL: false,
				},
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		{
			ResourceType: RegionHealthSourceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "healthSources",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		// Region Health Aggregation Policy - when a whole backend service counts
		// as healthy, for cross-region load balancing. NOTE the URL segment is
		// "healthAggregationPolicies" while the method group is
		// "regionHealthAggregationPolicies". ponytail: patch needs the usual
		// fingerprint dance; deferred, so a change replaces.
		{
			ResourceType: RegionHealthAggregationPolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "healthAggregationPolicies",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		// Region Network Firewall Policy - the regional twin. Same
		// firewallPolicies path segment, same fingerprint-on-patch and
		// no-name-in-patch-body constraints.
		{
			ResourceType: RegionNetworkFirewallPolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "firewallPolicies",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPatch,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       true,
					FieldName:     "fingerprint",
					LocationInURL: false,
				},
			},
			RequestTransformer:  base.DropFieldsOnUpdate("name"),
			ResponseTransformer: base.RegionResponseTransformer,
		},

		// Network Firewall Policy - the modern replacement for VPC firewall
		// rules. NOTE the URL segment is "firewallPolicies" while the API method
		// group is "networkFirewallPolicies". patch needs the current
		// fingerprint. Rules and network associations are separate API
		// operations (addRule / addAssociation) and are not modelled yet — the
		// API rejects them inline ("Rules must be added using the addRule
		// method"). The server-populated "rules" field is deliberately absent
		// from the schema so GCP's implied rules do not read back as drift.
		{
			ResourceType: NetworkFirewallPolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "firewallPolicies",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPatch,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       true,
					FieldName:     "fingerprint",
					LocationInURL: false,
				},
			},
			// The patch endpoint refuses a body carrying "name": "Can only
			// change description and rules of the firewall policy with patch
			// operation."
			RequestTransformer:  base.DropFieldsOnUpdate("name"),
			ResponseTransformer: nil,
		},

		// HTTPS Health Check - the legacy check's TLS twin, for target pools
		// whose backends terminate TLS. PATCH needs no fingerprint either.
		{
			ResourceType: HttpsHealthCheckResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "httpsHealthChecks",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate:    true,
				UpdateMethod:      base.UpdateMethodPatch,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// HTTP Health Check - the legacy global check. Only targetPool accepts
		// these; everything else uses healthChecks/regionHealthChecks. PATCH
		// needs no fingerprint (verified against the live API).
		{
			ResourceType: HttpHealthCheckResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "httpHealthChecks",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate:    true,
				UpdateMethod:      base.UpdateMethodPatch,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// External VPN Gateway - global description of the peer side of a VPN.
		{
			ResourceType: ExternalVpnGatewayResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "externalVpnGateways",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				// ponytail: only labels are mutable, via setLabels.
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},
		// HA VPN Gateway - the Google side. GCP assigns the interface IPs.
		{
			ResourceType: HaVpnGatewayResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "vpnGateways",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},
		// VPN Tunnel - one IPsec tunnel between a gateway interface pair.
		// ponytail: no vpnTunnels.patch exists, so a change replaces.
		// Target VPN Gateway - the classic (route-based) VPN gateway, a separate
		// collection from the HA gateway above and drawing on a separate quota.
		// Immutable: PATCH is not a method on it at all.
		{
			ResourceType: TargetVpnGatewayResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "targetVpnGateways",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		{
			ResourceType: VpnTunnelResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "vpnTunnels",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.ResponseTransformerFunc(vpnTunnelResponseTransformer),
		},

		// Region Disk - a persistent disk replicated across two zones in a
		// region, for workloads that must survive a zone outage.
		{
			ResourceType: RegionDiskResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "disks",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				// ponytail: resize and setLabels are separate endpoints, as with
				// the zonal Disk; wire them when someone needs them.
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  base.RequestTransformerFunc(regionDiskRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(regionDiskResponseTransformer),
		},

		// Region SSL Policy - the regional twin, for regional target HTTPS
		// proxies, which cannot reference the global policy. Same
		// fingerprint-on-patch requirement.
		{
			ResourceType: RegionSslPolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "sslPolicies",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPatch,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       true,
					FieldName:     "fingerprint",
					LocationInURL: false,
				},
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		// SSL Policy - global TLS floor for target HTTPS/SSL proxies.
		// sslPolicies.patch rejects a body without the current fingerprint
		// ("Required field 'resource.fingerprint' not specified"), so updates
		// go through optimistic locking like firewalls and subnetworks.
		{
			ResourceType: SslPolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "sslPolicies",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPatch,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       true,
					FieldName:     "fingerprint",
					LocationInURL: false,
				},
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// Node Template - regional spec a sole-tenant node group stamps nodes
		// from. Free on its own; the group reserves the hardware. ponytail: no
		// patch endpoint, so a change replaces. "serverBinding" and "status" are
		// server-populated and deliberately absent from the schema.
		{
			ResourceType: NodeTemplateResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "nodeTemplates",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		// Region Instant Snapshot - the regional twin, taken from a regional
		// disk, so it survives losing a zone. Same SSD-class source requirement.
		// Region Snapshot - a regional incremental backup, stored in the same
		// region as its source. Distinct from the global Snapshot above: that
		// one lives at /global/snapshots, this one at /regions/{r}/snapshots.
		// ponytail: as with the global snapshot, only setLabels mutates one, so
		// update replaces.
		{
			ResourceType: RegionSnapshotResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "snapshots",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		{
			ResourceType: RegionInstantSnapshotResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "instantSnapshots",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		// Instant Snapshot - zonal, same-zone fast rollback point. Needs an
		// SSD-class source disk. ponytail: only setLabels mutates one, so update
		// replaces.
		{
			ResourceType: InstantSnapshotResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "instantSnapshots",
				Scope: &base.ScopeConfig{
					Type: base.ScopeZonal,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.ZoneResponseTransformer,
		},

		// Snapshot - global incremental disk backup. ponytail: setLabels needs
		// a labelFingerprint round-trip and nothing else is mutable, so update
		// replaces.
		{
			ResourceType: SnapshotResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "snapshots",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// Machine Image - global whole-VM capture (all disks plus instance
		// config). ponytail: only setLabels mutates one, so update replaces.
		{
			ResourceType: MachineImageResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "machineImages",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// Image - global bootable image, normally captured from a disk.
		// ponytail: images.patch only moves labels/description and needs a
		// labelFingerprint round-trip, so update replaces instead.
		{
			ResourceType: ImageResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "images",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// Resource Policy - regional snapshot/instance schedule attached to
		// disks. ponytail: patch is beta-only, so update replaces.
		{
			ResourceType: ResourcePolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "resourcePolicies",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		// Backend Bucket - global LB backend that serves a GCS bucket. PATCH
		// takes the name as a path segment, so the generic engine handles update.
		{
			ResourceType: BackendBucketResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "backendBuckets",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate:    true,
				UpdateMethod:      base.UpdateMethodPatch,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// Region Autoscaler - scales a RegionInstanceGroupManager. Same
		// ?autoscaler=NAME patch quirk as the zonal one, so update is off.
		{
			ResourceType: RegionAutoscalerResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "autoscalers",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  autoscalerRequestTransformer,
			ResponseTransformer: regionAutoscalerResponseTransformer,
		},

		// Autoscaler - zonal, scales an InstanceGroupManager on a utilization
		// signal. ponytail: update is off because autoscalers.patch takes the
		// name as ?autoscaler=NAME, which the generic URL builder cannot emit.
		{
			ResourceType: AutoscalerResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "autoscalers",
				Scope: &base.ScopeConfig{
					Type: base.ScopeZonal,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  autoscalerRequestTransformer,
			ResponseTransformer: autoscalerResponseTransformer,
		},

		// Region Instance Group Manager - a MIG spread across zones of a region,
		// which is what production normally wants. Same immutability caveat as
		// the zonal MIG.
		{
			ResourceType: RegionInstanceGroupManagerResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "instanceGroupManagers",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		// Instance Group Manager - zonal MIG that stamps VMs out of an
		// InstanceTemplate. ponytail: patch (template/targetSize/namedPorts)
		// deferred until the update body is verified, so changes replace.
		{
			ResourceType: InstanceGroupManagerResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "instanceGroupManagers",
				Scope: &base.ScopeConfig{
					Type: base.ScopeZonal,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.ZoneResponseTransformer,
		},

		// Region Instance Template - the regional twin, preferred for a regional
		// MIG so its dependency stays inside the region. Immutable, like the
		// global one.
		{
			ResourceType: RegionInstanceTemplateResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "instanceTemplates",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		// Instance Template - immutable global VM blueprint for managed instance
		// groups. GCP has no update method, so every change is a replace.
		{
			ResourceType: InstanceTemplateResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "instanceTemplates",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate:    false, // instanceTemplates are immutable
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},
		{
			ResourceType: RegionSecurityPolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "securityPolicies",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				// ponytail: as with the global policy, update needs fingerprint
				// optimistic locking plus per-rule verbs; defer until verified.
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},
		{
			ResourceType: SecurityPolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "securityPolicies",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				// ponytail: update needs fingerprint optimistic locking; defer until verified
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// Health Check - Global health checks for backend services
		{
			ResourceType: HealthCheckResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "healthChecks",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate:    true,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// Backend Service - Global backend service for load balancers
		{
			ResourceType: BackendServiceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "backendServices",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate: true,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       true,
					FieldName:     "fingerprint",
					LocationInURL: false,
				},
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// URL Map - Global URL routing rules
		{
			ResourceType: UrlMapResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "urlMaps",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate: true,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       true,
					FieldName:     "fingerprint",
					LocationInURL: false,
				},
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// Target gRPC Proxy - Global proxy for proxyless gRPC service mesh
		{
			ResourceType: TargetGrpcProxyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "targetGrpcProxies",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate: true,
				// Unlike the other target proxies, patch rejects a request
				// without a fingerprint.
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       true,
					FieldName:     "fingerprint",
					LocationInURL: false,
				},
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// Target HTTP Proxy - Global HTTP proxy for load balancers
		{
			ResourceType: TargetHttpProxyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "targetHttpProxies",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate:    true,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// Target HTTPS Proxy - Global HTTPS proxy for load balancers
		{
			ResourceType: TargetHttpsProxyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "targetHttpsProxies",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate:    true,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// Target TCP Proxy - Global TCP proxy for load balancers
		// ponytail: update is off. targetTcpProxies has no patch and no update - only setBackendService
		// and setProxyHeader, so a PATCH would go to a URL the
		// API does not serve. A change replaces.
		{
			ResourceType: TargetTcpProxyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "targetTcpProxies",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// Target SSL Proxy - Global SSL proxy for load balancers
		// ponytail: update is off. targetSslProxies has no patch and no update - only setBackendService,
		// setCertificateMap, setProxyHeader, setSslCertificates and setSslPolicy, so a PATCH would go to a URL the
		// API does not serve. A change replaces.
		{
			ResourceType: TargetSslProxyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "targetSslProxies",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// Global Forwarding Rule - Entry point for global load balancers
		{
			ResourceType: GlobalForwardingRuleResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "forwardingRules",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate: true,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       true,
					FieldName:     "labelFingerprint",
					LocationInURL: false,
				},
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// ==================== Load Balancer - Regional Resources ====================

		// Region Health Check - Regional health checks for backend services
		{
			ResourceType: RegionHealthCheckResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "healthChecks",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    true,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		// Region Backend Service - Regional backend service for internal load balancers
		{
			ResourceType: RegionBackendServiceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "backendServices",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate: true,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       true,
					FieldName:     "fingerprint",
					LocationInURL: false,
				},
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		// Region URL Map - Regional URL routing rules
		{
			ResourceType: RegionUrlMapResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "urlMaps",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate: true,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       true,
					FieldName:     "fingerprint",
					LocationInURL: false,
				},
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		// Region Target HTTP Proxy - Regional HTTP proxy for internal load balancers
		{
			ResourceType: RegionTargetHttpProxyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "targetHttpProxies",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    true,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		// Region Target HTTPS Proxy - Regional HTTPS proxy for internal load balancers
		{
			ResourceType: RegionTargetHttpsProxyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "targetHttpsProxies",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    true,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		// Region Target TCP Proxy - Regional TCP proxy for internal proxy load balancers
		// ponytail: update is off. regionTargetTcpProxies has only delete, get, insert and list, so a PATCH would go to a URL the
		// API does not serve. A change replaces.
		{
			ResourceType: RegionTargetTcpProxyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "targetTcpProxies",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		// Forwarding Rule - Regional entry point for load balancers
		{
			ResourceType: ForwardingRuleResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "forwardingRules",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate: true,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       true,
					FieldName:     "labelFingerprint",
					LocationInURL: false,
				},
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},

		// Target Pool - Classic target pool for network load balancers.
		//
		// ponytail: update is off. targetPools has no patch and no update in the
		// compute API - only addHealthCheck, addInstance, removeHealthCheck,
		// removeInstance, setBackup and setSecurityPolicy - so a PATCH would go
		// to a URL the API does not serve. It was registered as updatable, and
		// no conformance case ever exercised it.
		{
			ResourceType: TargetPoolResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "targetPools",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    false,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.RegionResponseTransformer,
		},
	})

	if err != nil {
		panic(err)
	}

	registry.Register(
		RouterNatResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(cfg *config.Config) prov.Provisioner {
			return NewRouterNatProvisioner(cfg)
		},
	)
}
