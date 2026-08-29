// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigtable

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
	InstanceResourceType = "GCP::Bigtable::Instance"
	ClusterResourceType  = "GCP::Bigtable::Cluster"
	TableResourceType    = "GCP::Bigtable::Table"
)

// bigtableRegistry is the unified registry for all Bigtable resources
var bigtableRegistry *base.ResourceRegistry

func NewBigtableProvisioner(cfg *config.Config, resourceType string) (prov.Provisioner, error) {
	if bigtableRegistry == nil {
		return nil, fmt.Errorf("bigtable registry not initialized")
	}

	def, ok := bigtableRegistry.Definitions[resourceType]
	if !ok {
		return nil, fmt.Errorf("no configuration found for resource type: %s", resourceType)
	}

	// Create BaseResource
	baseResource := &base.BaseResource{
		Config:              cfg,
		APIConfig:           BigtableAPI,
		OperationConfig:     BigtableOperations,
		ResourceConfig:      def.ResourceConfig,
		NativeIDConfig:      BigtableNativeID,
		RequestTransformer:  def.RequestTransformer,
		ResponseTransformer: def.ResponseTransformer,
	}

	// Wrap with Bigtable-specific provisioner to handle Create query parameters
	return newBigtableProvisionerWithBase(baseResource, def.ResourceConfig.ResourceType), nil
}

// Wrapper functions to adapt transformers to base interfaces
func wrapInstanceBodyBuilder(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	return instanceBodyBuilderWithContext(props, ctx.Project)
}

func wrapClusterBodyBuilder(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	return clusterBodyBuilder(props, ctx.Project)
}

func wrapTableBodyBuilder(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	return tableBodyBuilder(props)
}

func init() {
	// Create the registry with common Bigtable API configurations
	bigtableRegistry = base.NewResourceRegistry(
		BigtableAPI,
		BigtableOperations,
		BigtableNativeID,
	)

	// Register all Bigtable resources
	err := bigtableRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: InstanceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "instances",
				Scope:          nil, // Instances are project-level resources
				ParentResource: nil, // Top-level resource
				SupportsUpdate: false,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       false,
					FieldName:     "",
					LocationInURL: false,
				},
				RequestWrapper: "", // Instance builder handles wrapping internally due to special structure
			},
			RequestTransformer:  base.RequestTransformerFunc(wrapInstanceBodyBuilder),
			ResponseTransformer: base.ResponseTransformerFunc(instanceResponseTransformer),
		},
		{
			ResourceType: ClusterResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "clusters",
				Scope:        nil,
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "instances",
					PropertyName:   "instance",
					RequiresParent: true,
				},
				SupportsUpdate: false,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       false,
					FieldName:     "",
					LocationInURL: false,
				},
				RequestWrapper: "", // Cluster API doesn't require wrapping
			},
			RequestTransformer:  base.RequestTransformerFunc(wrapClusterBodyBuilder),
			ResponseTransformer: base.ResponseTransformerFunc(clusterResponseTransformer),
		},
		{
			ResourceType: TableResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "tables",
				Scope:        nil,
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "instances",
					PropertyName:   "instance",
					RequiresParent: true,
				},
				SupportsUpdate: false,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       false,
					FieldName:     "",
					LocationInURL: false,
				},
				RequestWrapper: "table", // Bigtable API expects payload wrapped in "table"
			},
			RequestTransformer:  base.RequestTransformerFunc(wrapTableBodyBuilder),
			ResponseTransformer: base.ResponseTransformerFunc(tableResponseTransformer),
		},
	})

	if err != nil {
		panic(err)
	}

	// Override registrations with BigtableProvisioner for proper Create handling
	// BigtableProvisioner adds the required instance_id/cluster_id/table_id query parameters
	bigtableResourceTypes := []string{InstanceResourceType, ClusterResourceType, TableResourceType}
	for _, rt := range bigtableResourceTypes {
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
				provisioner, _ := NewBigtableProvisioner(cfg, resourceType)
				return provisioner
			},
		)
	}
}
