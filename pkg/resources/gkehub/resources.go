// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package gkehub

import (
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const (
	MembershipResourceType = "GCP::GKEHub::Membership"
	FeatureResourceType    = "GCP::GKEHub::Feature"
)

var gkehubRegistry *base.ResourceRegistry

// NewGKEHubProvisioner creates a provisioner for a GKE Hub resource type
func NewGKEHubProvisioner(cfg *config.Config, resourceType string) (prov.Provisioner, error) {
	if gkehubRegistry == nil {
		return nil, fmt.Errorf("gke hub registry not initialized")
	}

	def, ok := gkehubRegistry.Definitions[resourceType]
	if !ok {
		return nil, fmt.Errorf("no configuration found for resource type: %s", resourceType)
	}

	baseResource := &base.BaseResource{
		Config:              cfg,
		APIConfig:           GKEHubAPI,
		OperationConfig:     GKEHubOperations,
		ResourceConfig:      def.ResourceConfig,
		NativeIDConfig:      GKEHubNativeID,
		RequestTransformer:  def.RequestTransformer,
		ResponseTransformer: def.ResponseTransformer,
	}

	return newGKEHubProvisioner(baseResource, def.ResourceConfig.ResourceType), nil
}

func wrapMembershipBodyBuilder(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
	return membershipBodyBuilder(props)
}

func wrapMembershipResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	return membershipResponseTransformer(apiResponse, ctx)
}

func wrapFeatureBodyBuilder(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
	return featureBodyBuilder(props)
}

func wrapFeatureResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	return featureResponseTransformer(apiResponse, ctx)
}

func init() {
	gkehubRegistry = base.NewResourceRegistry(
		GKEHubAPI,
		GKEHubOperations,
		GKEHubNativeID,
	)

	err := gkehubRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: MembershipResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "memberships",
				Scope:          &base.ScopeConfig{Type: base.ScopeLocationBased},
				SupportsUpdate: true,
			},
			RequestTransformer:  base.RequestTransformerFunc(wrapMembershipBodyBuilder),
			ResponseTransformer: base.ResponseTransformerFunc(wrapMembershipResponseTransformer),
		},
		{
			ResourceType: FeatureResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "features",
				Scope:          &base.ScopeConfig{Type: base.ScopeLocationBased},
				SupportsUpdate: true,
			},
			RequestTransformer:  base.RequestTransformerFunc(wrapFeatureBodyBuilder),
			ResponseTransformer: base.ResponseTransformerFunc(wrapFeatureResponseTransformer),
		},
	})

	if err != nil {
		panic(err)
	}

	// Override the auto-registered provisioners from RegisterAll with our custom ones
	// that handle the membershipId/featureId query parameters on create
	gkehubResourceTypes := []string{MembershipResourceType, FeatureResourceType}
	for _, rt := range gkehubResourceTypes {
		resourceType := rt
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
				provisioner, _ := NewGKEHubProvisioner(cfg, resourceType)
				return provisioner
			},
		)
	}
}
