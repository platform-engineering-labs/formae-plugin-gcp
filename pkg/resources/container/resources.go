// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package container

import (
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
)

// Resource type constants
const (
	ClusterResourceType  = "GCP::Container::Cluster"
	NodePoolResourceType = "GCP::Container::NodePool"
)

// containerRegistry is the unified registry for all Container resources
var containerRegistry *base.ResourceRegistry

func NewContainerProvisioner(cfg *config.Config, resourceType string) (prov.Provisioner, error) {
	if containerRegistry == nil {
		return nil, fmt.Errorf("container registry not initialized")
	}

	_, ok := containerRegistry.Definitions[resourceType]
	if !ok {
		return nil, fmt.Errorf("no configuration found for resource type: %s", resourceType)
	}

	// Use the registry's provisioner creation
	return containerRegistry.CreateProvisioner(cfg, resourceType), nil
}

// Wrapper functions to adapt transformers to base interfaces
func wrapClusterResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	return clusterResponseTransformer(apiResponse, ctx.Location)
}

func wrapNodePoolResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	return nodePoolResponseTransformer(apiResponse, ctx.Location)
}

func wrapNodePoolBodyBuilder(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	return nodePoolBodyBuilder(props)
}

func init() {
	// Create the registry with common Container API configurations
	containerRegistry = base.NewResourceRegistry(
		ContainerAPI,
		ContainerOperations,
		ContainerNativeID,
	)

	// Register all Container resources
	err := containerRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: ClusterResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "clusters",
				Scope:          &base.ScopeConfig{Type: base.ScopeLocationBased},
				ParentResource: nil, // Top-level resource
				SupportsUpdate: false,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       false,
					FieldName:     "",
					LocationInURL: false,
				},
				RequestWrapper: "cluster", // GKE API requires wrapping in "cluster" field
			},
			RequestTransformer:  nil, // Pass through properties
			ResponseTransformer: base.ResponseTransformerFunc(wrapClusterResponseTransformer),
		},
		{
			ResourceType: NodePoolResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "nodePools",
				Scope:        &base.ScopeConfig{Type: base.ScopeLocationBased},
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "clusters",
					PropertyName:   "cluster", // Property name in node pool is "cluster" (singular)
					RequiresParent: true,
				},
				SupportsUpdate: false,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       false,
					FieldName:     "",
					LocationInURL: false,
				},
				RequestWrapper: "", // Body builder handles wrapping
			},
			RequestTransformer:  base.RequestTransformerFunc(wrapNodePoolBodyBuilder),
			ResponseTransformer: base.ResponseTransformerFunc(wrapNodePoolResponseTransformer),
		},
	})

	if err != nil {
		panic(err)
	}
}
