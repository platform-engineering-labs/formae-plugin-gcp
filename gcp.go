// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package main

import (
	"context"
	"encoding/json"

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
type Plugin struct {
	// oidc carries the token source the SDK installs via SetOidcTokenSource,
	// plus the plugin-lifetime token-source cache it backs. Nil until the SDK
	// calls SetOidcTokenSource (or on an agent too old to pair a broker), in
	// which case every target config threads nil deps and Oidc auth fails
	// closed rather than falling back to ambient credentials.
	oidc *config.OidcDeps
}

// Compile-time check: Plugin must satisfy ResourcePlugin interface.
var _ plugin.ResourcePlugin = &Plugin{}

// Compile-time check: Plugin must satisfy OidcAware, so the SDK hands it an
// OidcTokenSource at startup.
var _ plugin.OidcAware = &Plugin{}

// SetOidcTokenSource receives the token source the SDK mints OIDC identity
// tokens through. It is called once at startup, before any operation, and
// threads the resulting deps onto every parsed target config so Oidc auth
// blocks can exchange a token for Google credentials.
func (p *Plugin) SetOidcTokenSource(src plugin.OidcTokenSource) {
	p.oidc = config.NewOidcDeps(src)
}

// targetConfig parses a request's target config with this plugin instance's
// OIDC deps attached.
func (p *Plugin) targetConfig(raw json.RawMessage) *config.Config {
	return config.FromTargetConfig(raw, p.oidc)
}

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
		MaxRequestsPerSecondForNamespace: 10,
	}
}

// DiscoveryFilters returns declarative filters for excluding resources from discovery.
// Uses RFC 9535 JSONPath with match() regex function to filter GKE Autopilot-managed resources.
func (p *Plugin) DiscoveryFilters() []model.MatchFilter {
	// GKE Autopilot resources are identified by labels like goog-gke-node, goog-gke-volume, etc.
	// We filter these out during discovery to avoid managing resources that GKE controls.
	return []model.MatchFilter{
		{
			// Anything formae created in order to run in this project or to
			// reach it, chiefly an agent's own substrate. Discovered like any
			// other resource, it could be imported and then reconciled away,
			// which severs formae's own access.
			//
			// The marker answers one question, whether formae created the
			// thing. It says nothing about who may delete it. The key avoids a
			// colon so the same spelling is legal here as on AWS and Azure:
			// GCP label keys admit only lowercase letters, digits, underscores
			// and hyphens.
			Conditions: []model.FilterCondition{
				{
					PropertyPath:  `$.labels['formae-owned']`,
					PropertyValue: "true",
				},
			},
		},
		{
			// Project IAM bindings carry no labels, so connect's own grants are
			// recognised by the member string. It names both formae's shared
			// pool and its subject namespace, and requiring the two together
			// keeps a project that happens to reuse the pool id from matching.
			//
			// The condition searches the whole document rather than $.member
			// because a filter selector does not evaluate against a scalar.
			// Scoping this filter to the one type is what makes that safe: a
			// ProjectIamMember's only other fields are project and role, and
			// neither can hold a pool path.
			ResourceTypes: []string{"GCP::IAM::ProjectIamMember"},
			Conditions: []model.FilterCondition{
				{
					PropertyPath:  `$[?search(@, "workloadIdentityPools/formae-ai/subject/fai:")]`,
					PropertyValue: "", // Any value matches (existence check)
				},
			},
		},
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

// stripNonFailureMessage clears StatusMessage on any non-failure result. The CLI
// renders StatusMessage as a resource's "reason:", which should explain a failure
// — not narrate normal progress ("networks creation in progress") or success. So
// a reason is only surfaced when the operation actually failed.
func stripNonFailureMessage(pr *resource.ProgressResult) {
	if pr != nil && pr.OperationStatus != resource.OperationStatusFailure {
		pr.StatusMessage = ""
	}
}

// Create provisions a new GCP resource.
func (p *Plugin) Create(ctx context.Context, request *resource.CreateRequest) (res *resource.CreateResult, err error) {
	defer func() {
		if res != nil {
			stripNonFailureMessage(res.ProgressResult)
		}
	}()

	targetConfig := p.targetConfig(request.TargetConfig)

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
		provisioner := registry.Get(request.ResourceType, resource.OperationRead, p.targetConfig(request.TargetConfig))
		return provisioner.Read(ctx, request)
	}

	client, err := gcp.NewClient(ctx, p.targetConfig(request.TargetConfig))
	if err != nil {
		return nil, err
	}

	return client.ReadResource(ctx, request)
}

// Update modifies an existing GCP resource.
func (p *Plugin) Update(ctx context.Context, request *resource.UpdateRequest) (res *resource.UpdateResult, err error) {
	defer func() {
		if res != nil {
			stripNonFailureMessage(res.ProgressResult)
		}
	}()

	if registry.HasProvisioner(request.ResourceType, resource.OperationUpdate) {
		provisioner := registry.Get(request.ResourceType, resource.OperationUpdate, p.targetConfig(request.TargetConfig))
		return provisioner.Update(ctx, request)
	}

	client, err := gcp.NewClient(ctx, p.targetConfig(request.TargetConfig))
	if err != nil {
		return nil, err
	}

	return client.UpdateResource(ctx, request)
}

// Delete removes a GCP resource.
func (p *Plugin) Delete(ctx context.Context, request *resource.DeleteRequest) (res *resource.DeleteResult, err error) {
	defer func() {
		if res != nil {
			stripNonFailureMessage(res.ProgressResult)
		}
	}()

	if registry.HasProvisioner(request.ResourceType, resource.OperationDelete) {
		provisioner := registry.Get(request.ResourceType, resource.OperationDelete, p.targetConfig(request.TargetConfig))
		return provisioner.Delete(ctx, request)
	}

	client, err := gcp.NewClient(ctx, p.targetConfig(request.TargetConfig))
	if err != nil {
		return nil, err
	}

	return client.DeleteResource(ctx, request)
}

// Status checks the progress of an async GCP operation.
func (p *Plugin) Status(ctx context.Context, request *resource.StatusRequest) (res *resource.StatusResult, err error) {
	defer func() {
		if res != nil {
			stripNonFailureMessage(res.ProgressResult)
		}
	}()

	if request.ResourceType != "" {
		if registry.HasProvisioner(request.ResourceType, resource.OperationCheckStatus) {
			provisioner := registry.Get(request.ResourceType, resource.OperationCheckStatus, p.targetConfig(request.TargetConfig))
			return provisioner.Status(ctx, request)
		}
	}

	client, err := gcp.NewClient(ctx, p.targetConfig(request.TargetConfig))
	if err != nil {
		return nil, err
	}

	return client.StatusResource(ctx, request, p.Read)
}

// List returns all resource identifiers of a given type for discovery.
func (p *Plugin) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	if registry.HasProvisioner(request.ResourceType, resource.OperationList) {
		provisioner := registry.Get(request.ResourceType, resource.OperationList, p.targetConfig(request.TargetConfig))
		return provisioner.List(ctx, request)
	}

	client, err := gcp.NewClient(ctx, p.targetConfig(request.TargetConfig))
	if err != nil {
		return nil, err
	}

	return client.ListResources(ctx, request)
}
