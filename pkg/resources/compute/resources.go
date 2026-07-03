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
	AddressResourceType        = "GCP::Compute::Address"
	DiskResourceType           = "GCP::Compute::Disk"
	FirewallResourceType       = "GCP::Compute::Firewall"
	InstanceResourceType       = "GCP::Compute::Instance"
	NetworkResourceType        = "GCP::Compute::Network"
	RouterResourceType         = "GCP::Compute::Router"
	RouterNatResourceType      = "GCP::Compute::RouterNat"
	SubnetworkResourceType     = "GCP::Compute::Subnetwork"
	RouteResourceType          = "GCP::Compute::Route"
	SecurityPolicyResourceType = "GCP::Compute::SecurityPolicy"

	// Load Balancer backends + certificate
	InstanceGroupResourceType        = "GCP::Compute::InstanceGroup"
	NetworkEndpointGroupResourceType = "GCP::Compute::NetworkEndpointGroup"
	SslCertificateResourceType       = "GCP::Compute::SslCertificate"

	// Load Balancer - Global resources
	GlobalAddressResourceType        = "GCP::Compute::GlobalAddress"
	HealthCheckResourceType          = "GCP::Compute::HealthCheck"
	BackendServiceResourceType       = "GCP::Compute::BackendService"
	UrlMapResourceType               = "GCP::Compute::UrlMap"
	TargetHttpProxyResourceType      = "GCP::Compute::TargetHttpProxy"
	TargetHttpsProxyResourceType     = "GCP::Compute::TargetHttpsProxy"
	TargetTcpProxyResourceType       = "GCP::Compute::TargetTcpProxy"
	TargetSslProxyResourceType       = "GCP::Compute::TargetSslProxy"
	GlobalForwardingRuleResourceType = "GCP::Compute::GlobalForwardingRule"

	// Load Balancer - Regional resources
	RegionHealthCheckResourceType      = "GCP::Compute::RegionHealthCheck"
	RegionBackendServiceResourceType   = "GCP::Compute::RegionBackendService"
	RegionUrlMapResourceType           = "GCP::Compute::RegionUrlMap"
	RegionTargetHttpProxyResourceType  = "GCP::Compute::RegionTargetHttpProxy"
	RegionTargetHttpsProxyResourceType = "GCP::Compute::RegionTargetHttpsProxy"
	RegionTargetTcpProxyResourceType   = "GCP::Compute::RegionTargetTcpProxy"
	ForwardingRuleResourceType         = "GCP::Compute::ForwardingRule"
	TargetPoolResourceType             = "GCP::Compute::TargetPool"
)

// computeRegistry is the unified registry for all Compute resources
var computeRegistry *base.ResourceRegistry

func NewComputeProvisioner(cfg *config.Config, resourceType string) (prov.Provisioner, error) {
	if resourceType == RouterNatResourceType {
		return NewRouterNatProvisioner(cfg), nil
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
			ResponseTransformer: base.RegionResponseTransformer,
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

		// Instance Group - Zonal unmanaged instance group (GCE VM backend).
		// namedPorts are mutable; instance membership is managed out of band.
		{
			ResourceType: InstanceGroupResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "instanceGroups",
				Scope: &base.ScopeConfig{
					Type: base.ScopeZonal,
				},
				SupportsUpdate:    false, // group object immutable; namedPorts via setNamedPorts (follow-up)
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: base.ZoneResponseTransformer,
		},

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
		{
			ResourceType: TargetTcpProxyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "targetTcpProxies",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate:    true,
				OptimisticLocking: nil,
			},
			RequestTransformer:  nil,
			ResponseTransformer: nil,
		},

		// Target SSL Proxy - Global SSL proxy for load balancers
		{
			ResourceType: TargetSslProxyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "targetSslProxies",
				Scope: &base.ScopeConfig{
					Type: base.ScopeGlobal,
				},
				SupportsUpdate:    true,
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
		{
			ResourceType: RegionTargetTcpProxyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "targetTcpProxies",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    true,
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

		// Target Pool - Classic target pool for network load balancers
		{
			ResourceType: TargetPoolResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "targetPools",
				Scope: &base.ScopeConfig{
					Type: base.ScopeRegional,
				},
				SupportsUpdate:    true,
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
