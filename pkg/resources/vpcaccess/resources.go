// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package vpcaccess implements GCP Serverless VPC Access resources.
package vpcaccess

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const ConnectorResourceType = "GCP::Vpcaccess::Connector"

var vpcAccessRegistry *base.ResourceRegistry

func init() {
	vpcAccessRegistry = base.NewResourceRegistry(
		VpcAccessAPI, VpcAccessOperations, VpcAccessNativeID)

	// ponytail: Update deferred (as ArtifactRegistry/DNS/scheduler do) until PATCH
	// is verified live. create/delete are the async paths this batch proves out.
	err := vpcAccessRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: ConnectorResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "connectors",
				Scope:         &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam: "connectorId", // id goes in ?connectorId=, not the body
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
