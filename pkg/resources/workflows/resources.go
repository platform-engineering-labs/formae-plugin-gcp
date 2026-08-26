// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package workflows implements GCP Workflows resources.
package workflows

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const WorkflowResourceType = "GCP::Workflows::Workflow"

var workflowsRegistry *base.ResourceRegistry

func init() {
	workflowsRegistry = base.NewResourceRegistry(WorkflowsAPI, WorkflowsOperations, WorkflowsNativeID)

	err := workflowsRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: WorkflowResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "workflows",
				Scope:              &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam:      "workflowId", // id goes in ?workflowId=, not the body
				SupportsUpdate:     true,
				UpdateMaskFromBody: true, // PATCH ?updateMask=<body fields>
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationUpdate,
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
