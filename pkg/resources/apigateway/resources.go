// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package apigateway implements GCP API Gateway resources.
package apigateway

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const (
	ApiResourceType       = "GCP::ApiGateway::Api"
	ApiConfigResourceType = "GCP::ApiGateway::ApiConfig"
	GatewayResourceType   = "GCP::ApiGateway::Gateway"
)

var apiGatewayRegistry *base.ResourceRegistry

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
	apiGatewayRegistry = base.NewResourceRegistry(APIGatewayAPI, APIGatewayOperations, APIGatewayNativeID)

	err := apiGatewayRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: ApiResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "apis",
				// An api is always global; the path builder supplies the
				// location, so no location has to be named to list them.
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "apiId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
				ListItemsKey:       "apis",
			},
			Operations:          crudOperations(),
			RequestTransformer:  base.RequestTransformerFunc(requestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(responseTransformer),
		},
		{
			ResourceType: ApiConfigResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "configs",
				Scope:        &base.ScopeConfig{Type: base.ScopeGlobal},
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "apis",
					PropertyName:   "api",
					RequiresParent: true,
				},
				CreateIDParam:      "apiConfigId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
				ListItemsKey:       "apiConfigs",
			},
			Operations:          crudOperations(),
			RequestTransformer:  base.RequestTransformerFunc(requestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(responseTransformer),
		},
		{
			ResourceType: GatewayResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "gateways",
				// A gateway is the only regional resource here, but its region
				// is not the target's: API Gateway serves eleven regions, so a
				// gateway names its own and a parentless list spans them all
				// with the location wildcard. Binding it to the target's
				// location instead would hide every gateway outside it.
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "gatewayId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
				ListItemsKey:       "gateways",
			},
			Operations:          crudOperations(),
			RequestTransformer:  base.RequestTransformerFunc(requestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(responseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}

	// A config only exists underneath an api and there is no wildcard in the api
	// position, while discovery lists with no parent to name. Walk the apis.
	def := apiGatewayRegistry.Definitions[ApiConfigResourceType]
	registry.Register(ApiConfigResourceType, def.Operations, func(cfg *config.Config) prov.Provisioner {
		return &configWalkingProvisioner{
			BaseResource: &base.BaseResource{
				Config:              cfg,
				APIConfig:           APIGatewayAPI,
				OperationConfig:     APIGatewayOperations,
				ResourceConfig:      def.ResourceConfig,
				NativeIDConfig:      APIGatewayNativeID,
				RequestTransformer:  def.RequestTransformer,
				ResponseTransformer: def.ResponseTransformer,
			},
		}
	})
}
