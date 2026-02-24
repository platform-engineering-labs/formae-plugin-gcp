// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package cloudrun

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
	ServiceResourceType    = "GCP::CloudRun::Service"
	JobResourceType        = "GCP::CloudRun::Job"
	WorkerPoolResourceType = "GCP::CloudRun::WorkerPool"
	RevisionResourceType   = "GCP::CloudRun::Revision"
	ExecutionResourceType  = "GCP::CloudRun::Execution"
	TaskResourceType       = "GCP::CloudRun::Task"
)

// cloudRunRegistry is the unified registry for all Cloud Run resources
var cloudRunRegistry *base.ResourceRegistry

func NewCloudRunProvisioner(cfg *config.Config, resourceType string) (prov.Provisioner, error) {
	if cloudRunRegistry == nil {
		return nil, fmt.Errorf("cloud run registry not initialized")
	}

	def, ok := cloudRunRegistry.Definitions[resourceType]
	if !ok {
		return nil, fmt.Errorf("no configuration found for resource type: %s", resourceType)
	}

	// Create BaseResource directly (not using registry's CreateProvisioner which wraps it)
	baseResource := &base.BaseResource{
		Config:              cfg,
		APIConfig:           CloudRunAPI,
		OperationConfig:     CloudRunOperations,
		ResourceConfig:      def.ResourceConfig,
		NativeIDConfig:      CloudRunNativeID,
		RequestTransformer:  def.RequestTransformer,
		ResponseTransformer: def.ResponseTransformer,
	}

	// Wrap with Cloud Run-specific provisioner to handle Create query parameters
	return newCloudRunProvisionerWithBase(baseResource, def.ResourceConfig.ResourceType), nil
}

// Wrapper functions to adapt transformers to base interfaces
func wrapServiceBodyBuilder(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	return serviceBodyBuilder(props)
}

func wrapServiceResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	return serviceResponseTransformer(apiResponse, ctx)
}

func wrapJobBodyBuilder(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	return jobBodyBuilder(props)
}

func wrapJobResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	return jobResponseTransformer(apiResponse, ctx)
}

func wrapWorkerPoolBodyBuilder(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	return workerPoolBodyBuilder(props)
}

func wrapWorkerPoolResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	return workerPoolResponseTransformer(apiResponse, ctx)
}

func wrapRevisionResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	return revisionResponseTransformer(apiResponse, ctx)
}

func wrapExecutionResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	return executionResponseTransformer(apiResponse, ctx)
}

func wrapTaskResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	return taskResponseTransformer(apiResponse, ctx)
}

func init() {
	// Create the registry with common Cloud Run API configurations
	cloudRunRegistry = base.NewResourceRegistry(
		CloudRunAPI,
		CloudRunOperations,
		CloudRunNativeID,
	)

	// Register all Cloud Run resources
	err := cloudRunRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: ServiceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "services",
				Scope:          &base.ScopeConfig{Type: base.ScopeLocationBased},
				ParentResource: nil, // Top-level resource
				SupportsUpdate: false,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       false,
					FieldName:     "",
					LocationInURL: false,
				},
				RequestWrapper: "", // No wrapper needed for Cloud Run
			},
			RequestTransformer:  base.RequestTransformerFunc(wrapServiceBodyBuilder),
			ResponseTransformer: base.ResponseTransformerFunc(wrapServiceResponseTransformer),
		},
		{
			ResourceType: JobResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "jobs",
				Scope:          &base.ScopeConfig{Type: base.ScopeLocationBased},
				ParentResource: nil, // Top-level resource
				SupportsUpdate: false,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       false,
					FieldName:     "",
					LocationInURL: false,
				},
				RequestWrapper: "", // No wrapper needed for Cloud Run
			},
			RequestTransformer:  base.RequestTransformerFunc(wrapJobBodyBuilder),
			ResponseTransformer: base.ResponseTransformerFunc(wrapJobResponseTransformer),
		},
		{
			ResourceType: WorkerPoolResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "workerPools",
				Scope:          &base.ScopeConfig{Type: base.ScopeLocationBased},
				ParentResource: nil, // Top-level resource
				SupportsUpdate: false,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       false,
					FieldName:     "",
					LocationInURL: false,
				},
				RequestWrapper: "",
			},
			RequestTransformer:  base.RequestTransformerFunc(wrapWorkerPoolBodyBuilder),
			ResponseTransformer: base.ResponseTransformerFunc(wrapWorkerPoolResponseTransformer),
		},
		{
			ResourceType: RevisionResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "revisions",
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "services",
					PropertyName:   "service",
					RequiresParent: true,
				},
				SupportsUpdate: false,
			},
			Operations: []resource.Operation{
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			ResponseTransformer: base.ResponseTransformerFunc(wrapRevisionResponseTransformer),
		},
		{
			ResourceType: ExecutionResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "executions",
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "jobs",
					PropertyName:   "job",
					RequiresParent: true,
				},
				SupportsUpdate: false,
			},
			Operations: []resource.Operation{
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			ResponseTransformer: base.ResponseTransformerFunc(wrapExecutionResponseTransformer),
		},
		{
			ResourceType: TaskResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "tasks",
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "executions",
					PropertyName:   "execution",
					RequiresParent: true,
				},
				SupportsUpdate: false,
			},
			Operations: []resource.Operation{
				resource.OperationRead,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			ResponseTransformer: base.ResponseTransformerFunc(wrapTaskResponseTransformer),
		},
	})

	if err != nil {
		panic(err)
	}

	// Register CloudRunProvisioner with global registry for proper Create handling
	// CloudRunProvisioner adds the required serviceId/jobId query parameters
	cloudRunResourceTypes := []string{ServiceResourceType, JobResourceType, WorkerPoolResourceType}
	for _, rt := range cloudRunResourceTypes {
		resourceType := rt // capture by value for closure
		registry.Register(
			resourceType,
			[]resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationUpdate,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			func(cfg *config.Config) prov.Provisioner {
				provisioner, _ := NewCloudRunProvisioner(cfg, resourceType)
				return provisioner
			},
		)
	}
}
