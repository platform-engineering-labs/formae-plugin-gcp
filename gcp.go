// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package main

import (
	"context"

	"github.com/platform-engineering-labs/formae/pkg/model"
	"github.com/platform-engineering-labs/formae/pkg/plugin"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/gcp"
	_ "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
)

// Plugin implements the Formae ResourcePlugin interface for GCP.
// The SDK automatically provides identity methods (Name, Version, Namespace)
// by reading formae-plugin.pkl at startup.
type Plugin struct{}

// Compile-time check: Plugin must satisfy ResourcePlugin interface.
var _ plugin.ResourcePlugin = &Plugin{}

// GKEAutopilotResourceTypes lists GCP resource types that GKE Autopilot manages
var GKEAutopilotResourceTypes = []string{
	"GCP::Compute::Instance",       // Worker nodes
	"GCP::Compute::Disk",           // Persistent disks
	"GCP::Compute::Network",        // VPC network
	"GCP::Compute::Subnetwork",     // Subnets
	"GCP::Compute::Firewall",       // Firewall rules
	"GCP::Compute::BackendService", // Load balancer backends
	"GCP::Compute::ForwardingRule", // Load balancer forwarding rules
	"GCP::IAM::ServiceAccount",     // Service accounts for pods
	"GCP::Storage::Bucket",         // If using GCS for storage
}

// =============================================================================
// Configuration Methods
// =============================================================================

// RateLimit returns the rate limiting configuration for this plugin.
func (p *Plugin) RateLimit() model.RateLimitConfig {
	return model.RateLimitConfig{
		Scope:                            model.RateLimitScopeNamespace,
		MaxRequestsPerSecondForNamespace: 1,
	}
}

// DiscoveryFilters returns declarative filters for excluding resources from discovery.
// Uses RFC 9535 JSONPath with match() regex function to filter GKE Autopilot-managed resources.
func (p *Plugin) DiscoveryFilters() []model.MatchFilter {
	// GKE Autopilot resources are identified by labels like goog-gke-node, goog-gke-volume, etc.
	// We filter these out during discovery to avoid managing resources that GKE controls.
	return []model.MatchFilter{
		{
			ResourceTypes: GKEAutopilotResourceTypes,
			Conditions: []model.FilterCondition{
				{
					PropertyPath:  `$.labels[?match(@, "goog-gke-.*|gke-autopilot")]`,
					PropertyValue: "", // Any value matches (existence check)
				},
			},
		},
	}
}

// LabelConfig returns the label extraction configuration for discovered GCP resources.
// GCP resources typically use the "name" property as their identifier.
func (p *Plugin) LabelConfig() model.LabelConfig {
	return model.LabelConfig{
		DefaultQuery:      "$.name",
		ResourceOverrides: map[string]string{
			// Most GCP resources use "name" property, add overrides here if needed
		},
	}
}

// =============================================================================
// CRUD Operations
// =============================================================================

// Create provisions a new GCP resource.
func (p *Plugin) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	targetConfig := config.FromTargetConfig(request.TargetConfig)

	// Check for custom provisioner
	if registry.HasProvisioner(request.ResourceType, resource.OperationCreate) {
		provisioner := registry.Get(request.ResourceType, resource.OperationCreate, targetConfig)
		return provisioner.Create(ctx, request)
	}

	// Use generic GCP client
	client, err := gcp.NewClient(ctx, targetConfig)
	if err != nil {
		return nil, err
	}

	return client.CreateResource(ctx, request)
}

// Read retrieves the current state of a GCP resource.
func (p *Plugin) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	if registry.HasProvisioner(request.ResourceType, resource.OperationRead) {
		provisioner := registry.Get(request.ResourceType, resource.OperationRead, config.FromTargetConfig(request.TargetConfig))
		return provisioner.Read(ctx, request)
	}

	client, err := gcp.NewClient(ctx, config.FromTargetConfig(request.TargetConfig))
	if err != nil {
		return nil, err
	}

	return client.ReadResource(ctx, request)
}

// Update modifies an existing GCP resource.
func (p *Plugin) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	if registry.HasProvisioner(request.ResourceType, resource.OperationUpdate) {
		provisioner := registry.Get(request.ResourceType, resource.OperationUpdate, config.FromTargetConfig(request.TargetConfig))
		return provisioner.Update(ctx, request)
	}

	client, err := gcp.NewClient(ctx, config.FromTargetConfig(request.TargetConfig))
	if err != nil {
		return nil, err
	}

	return client.UpdateResource(ctx, request)
}

// Delete removes a GCP resource.
func (p *Plugin) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	if registry.HasProvisioner(request.ResourceType, resource.OperationDelete) {
		provisioner := registry.Get(request.ResourceType, resource.OperationDelete, config.FromTargetConfig(request.TargetConfig))
		return provisioner.Delete(ctx, request)
	}

	client, err := gcp.NewClient(ctx, config.FromTargetConfig(request.TargetConfig))
	if err != nil {
		return nil, err
	}

	return client.DeleteResource(ctx, request)
}

// Status checks the progress of an async GCP operation.
func (p *Plugin) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	if request.ResourceType != "" {
		if registry.HasProvisioner(request.ResourceType, resource.OperationCheckStatus) {
			provisioner := registry.Get(request.ResourceType, resource.OperationCheckStatus, config.FromTargetConfig(request.TargetConfig))
			return provisioner.Status(ctx, request)
		}
	}

	client, err := gcp.NewClient(ctx, config.FromTargetConfig(request.TargetConfig))
	if err != nil {
		return nil, err
	}

	return client.StatusResource(ctx, request, p.Read)
}

// List returns all resource identifiers of a given type for discovery.
func (p *Plugin) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	if registry.HasProvisioner(request.ResourceType, resource.OperationList) {
		provisioner := registry.Get(request.ResourceType, resource.OperationList, config.FromTargetConfig(request.TargetConfig))
		return provisioner.List(ctx, request)
	}

	client, err := gcp.NewClient(ctx, config.FromTargetConfig(request.TargetConfig))
	if err != nil {
		return nil, err
	}

	return client.ListResources(ctx, request)
}
