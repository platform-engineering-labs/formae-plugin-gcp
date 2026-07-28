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
	workflowsRegistry = base.NewResourceRegistry(
		WorkflowsAPI, WorkflowsOperations, WorkflowsNativeID)

	// ponytail: Update deferred (as artifactregistry/DNS/scheduler do) until
	// PATCH is verified live. create/delete are the async paths this batch
	// proves out.
	err := workflowsRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: WorkflowResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "workflows",
				Scope:         &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam: "workflowId", // id goes in ?workflowId=, not the body
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
