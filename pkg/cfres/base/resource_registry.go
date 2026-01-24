// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package base

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
)

// ResourceDefinition defines a complete resource registration for any GCP API
type ResourceDefinition struct {
	// ResourceType is the Formae resource type (e.g., "GCP::Compute::Network")
	ResourceType string

	// APIConfig defines the API endpoint configuration
	APIConfig APIConfig

	// OperationConfig defines how operations work for this API
	OperationConfig OperationConfig

	// ResourceConfig defines the resource metadata
	ResourceConfig ResourceConfig

	// NativeIDConfig defines how native IDs are formatted
	NativeIDConfig NativeIDConfig

	// RequestTransformer transforms request properties (optional)
	RequestTransformer RequestTransformer

	// ResponseTransformer transforms response properties (optional)
	ResponseTransformer ResponseTransformer

	// Operations lists the supported operations (if nil, uses StandardOperations)
	Operations []resource.Operation
}

// StandardOperations is the default set of operations for most resources
var StandardOperations = []resource.Operation{
	resource.OperationCreate,
	resource.OperationRead,
	resource.OperationUpdate,
	resource.OperationDelete,
	resource.OperationList,
	resource.OperationCheckStatus,
}

// ResourceRegistry manages resource definitions for a specific API
type ResourceRegistry struct {
	// apiConfig is the common API configuration for all resources in this registry
	apiConfig APIConfig

	// operationConfig is the common operation configuration
	operationConfig OperationConfig

	// nativeIDConfig is the common native ID configuration
	nativeIDConfig NativeIDConfig

	// Definitions maps resource types to their definitions (exported for access)
	Definitions map[string]*ResourceDefinition
}

// NewResourceRegistry creates a new resource registry with common API configurations
func NewResourceRegistry(
	apiConfig APIConfig,
	operationConfig OperationConfig,
	nativeIDConfig NativeIDConfig,
) *ResourceRegistry {
	return &ResourceRegistry{
		apiConfig:       apiConfig,
		operationConfig: operationConfig,
		nativeIDConfig:  nativeIDConfig,
		Definitions:     make(map[string]*ResourceDefinition),
	}
}

// Register registers a resource definition with the registry
func (r *ResourceRegistry) Register(def ResourceDefinition) error {
	if def.ResourceType == "" {
		return fmt.Errorf("resource type cannot be empty")
	}

	// Use common configurations if not specified
	if def.APIConfig.BaseURL == "" {
		def.APIConfig = r.apiConfig
	}
	if def.OperationConfig.OperationIDExtractor == nil {
		def.OperationConfig = r.operationConfig
	}
	if def.NativeIDConfig.Format == "" {
		def.NativeIDConfig = r.nativeIDConfig
	}

	// Use standard operations if not specified
	if def.Operations == nil {
		def.Operations = StandardOperations
	}

	// Store the definition
	r.Definitions[def.ResourceType] = &def

	// Register with the global registry
	registry.Register(
		def.ResourceType,
		def.Operations,
		func(cfg *config.Config) prov.Provisioner {
			return r.CreateProvisioner(cfg, def.ResourceType)
		},
	)

	return nil
}

// RegisterAll registers multiple resource definitions at once
func (r *ResourceRegistry) RegisterAll(definitions []ResourceDefinition) error {
	for _, def := range definitions {
		if err := r.Register(def); err != nil {
			return fmt.Errorf("failed to register %s: %w", def.ResourceType, err)
		}
	}
	return nil
}

// CreateProvisioner creates a provisioner for a registered resource type (exported for access)
func (r *ResourceRegistry) CreateProvisioner(cfg *config.Config, resourceType string) prov.Provisioner {
	def, ok := r.Definitions[resourceType]
	if !ok {
		panic(fmt.Sprintf("no definition found for resource type: %s", resourceType))
	}

	baseResource := &BaseResource{
		Config:              cfg,
		APIConfig:           def.APIConfig,
		OperationConfig:     def.OperationConfig,
		ResourceConfig:      def.ResourceConfig,
		NativeIDConfig:      def.NativeIDConfig,
		RequestTransformer:  def.RequestTransformer,
		ResponseTransformer: def.ResponseTransformer,
	}

	return &UnifiedProvisioner{
		base: baseResource,
	}
}

// UnifiedProvisioner is a unified provisioner that works with any GCP API
type UnifiedProvisioner struct {
	base *BaseResource
}

// Ensure UnifiedProvisioner implements prov.Provisioner
var _ prov.Provisioner = &UnifiedProvisioner{}

// Create creates a new resource
func (p *UnifiedProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	return p.base.Create(ctx, request)
}

// Read reads an existing resource
func (p *UnifiedProvisioner) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	return p.base.Read(ctx, request)
}

// Update updates an existing resource
func (p *UnifiedProvisioner) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	return p.base.Update(ctx, request)
}

// Delete deletes a resource
func (p *UnifiedProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	return p.base.Delete(ctx, request)
}

// List lists resources
func (p *UnifiedProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	return p.base.List(ctx, request)
}

// Status checks operation status
func (p *UnifiedProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	result, err := p.base.Status(ctx, request)
	if err != nil {
		return nil, err
	}

	// For successful operations, read the resource to get properties
	if result.ProgressResult.OperationStatus == resource.OperationStatusSuccess &&
		result.ProgressResult.NativeID != "" {
		readResult, err := p.Read(ctx, &resource.ReadRequest{
			NativeID:     result.ProgressResult.NativeID,
			ResourceType: request.ResourceType,
			TargetConfig: request.TargetConfig,
		})
		if err == nil && readResult.ErrorCode == "" {
			result.ProgressResult.ResourceProperties = []byte(readResult.Properties)
		}
	}

	return result, nil
}
