// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package artifactregistry implements GCP Artifact Registry resources.
package artifactregistry

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const RepositoryResourceType = "GCP::ArtifactRegistry::Repository"
const RuleResourceType = "GCP::ArtifactRegistry::Rule"

var artifactRegistryRegistry *base.ResourceRegistry

func init() {
	artifactRegistryRegistry = base.NewResourceRegistry(
		ArtifactRegistryAPI, ArtifactRegistryOperations, ArtifactRegistryNativeID)

	// ponytail: Update deferred (as DNS/CloudRun/scheduler do) until PATCH is
	// verified live. create/delete are the async paths this batch proves out.
	err := artifactRegistryRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: RepositoryResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "repositories",
				Scope:         &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam: "repositoryId", // id goes in ?repositoryId=, not the body
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
		{
			// A rule gates pull/push on its repository. Unlike repositories,
			// rules are synchronous, so this definition carries its own
			// OperationConfig.
			ResourceType:    RuleResourceType,
			OperationConfig: ArtifactRegistryRuleOperations,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "rules",
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "repositories",
					PropertyName:   "repository",
					RequiresParent: true,
				},
				CreateIDParam:      "ruleId", // id goes in ?ruleId=, not the body
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
			RequestTransformer:  base.RequestTransformerFunc(ruleRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(ruleResponseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}
}
