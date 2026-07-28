// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package apigateway implements GCP API Gateway resources.
package apigateway

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const APIResourceType = "GCP::Apigateway::Api"

var apiGatewayRegistry *base.ResourceRegistry

func init() {
	apiGatewayRegistry = base.NewResourceRegistry(
		APIGatewayAPI, APIGatewayOperations, APIGatewayNativeID)

	// ponytail: Update deferred (as artifactregistry/DNS/CloudRun do) until PATCH
	// is verified live. create/delete are the async paths this batch proves out.
	err := apiGatewayRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: APIResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "apis",
				Scope:         &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam: "apiId", // id goes in ?apiId=, not the body
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
	})
	if err != nil {
		panic(err)
	}
}
