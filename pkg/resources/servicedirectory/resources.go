// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package servicedirectory implements GCP Service Directory resources.
package servicedirectory

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const (
	NamespaceResourceType = "GCP::ServiceDirectory::Namespace"
	ServiceResourceType   = "GCP::ServiceDirectory::Service"
	EndpointResourceType  = "GCP::ServiceDirectory::Endpoint"
)

var serviceDirectoryRegistry *base.ResourceRegistry

// crudOperations - every type here supports the full set; updates are a PATCH
// with an update mask built from the body.
func crudOperations() []resource.Operation {
	return []resource.Operation{
		resource.OperationCreate,
		resource.OperationRead,
		resource.OperationUpdate,
		resource.OperationDelete,
		resource.OperationList,
		resource.OperationCheckStatus,
	}
}

func init() {
	serviceDirectoryRegistry = base.NewResourceRegistry(
		ServiceDirectoryAPI,
		ServiceDirectoryOperations,
		ServiceDirectoryNativeID,
	)

	err := serviceDirectoryRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: NamespaceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "namespaces",
				Scope:              &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam:      "namespaceId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
				ListItemsKey:       "namespaces",
			},
			Operations:          crudOperations(),
			RequestTransformer:  base.RequestTransformerFunc(requestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(responseTransformer),
		},
		{
			ResourceType: ServiceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "services",
				Scope:        &base.ScopeConfig{Type: base.ScopeLocationBased},
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "namespaces",
					PropertyName:   "namespace",
					RequiresParent: true,
				},
				CreateIDParam:      "serviceId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
				ListItemsKey:       "services",
			},
			Operations:          crudOperations(),
			RequestTransformer:  base.RequestTransformerFunc(requestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(responseTransformer),
		},
		{
			ResourceType: EndpointResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "endpoints",
				Scope:        &base.ScopeConfig{Type: base.ScopeLocationBased},
				// An endpoint hangs off a namespace AND a service, carried
				// together as "{namespace}/{service}".
				ParentResource: &base.ParentResourceConfig{
					ParentType:         "namespaces",
					PropertyName:       "namespace",
					SecondPropertyName: "service",
					RequiresParent:     true,
					ParentPathSegments: []string{"namespaces", "services"},
				},
				CreateIDParam:      "endpointId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
				ListItemsKey:       "endpoints",
			},
			Operations:          crudOperations(),
			RequestTransformer:  base.RequestTransformerFunc(requestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(responseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}

	// Services and endpoints have no wildcard to list across their parents:
	// "namespaces/-" and "services/-" are both rejected with "Could not parse
	// namespace name". Discovery lists with no parent to name, so each walks the
	// level above it instead. See walking_list.go.
	for _, rt := range []string{ServiceResourceType, EndpointResourceType} {
		resourceType := rt // capture by value for closure
		def := serviceDirectoryRegistry.Definitions[resourceType]
		registry.Register(resourceType, def.Operations, func(cfg *config.Config) prov.Provisioner {
			return &walkingListProvisioner{
				BaseResource: &base.BaseResource{
					Config:              cfg,
					APIConfig:           ServiceDirectoryAPI,
					OperationConfig:     ServiceDirectoryOperations,
					ResourceConfig:      def.ResourceConfig,
					NativeIDConfig:      ServiceDirectoryNativeID,
					RequestTransformer:  def.RequestTransformer,
					ResponseTransformer: def.ResponseTransformer,
				},
			}
		})
	}
}
