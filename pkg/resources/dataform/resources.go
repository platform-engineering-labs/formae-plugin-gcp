// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dataform

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const (
	RepositoryResourceType     = "GCP::Dataform::Repository"
	WorkspaceResourceType      = "GCP::Dataform::Workspace"
	ReleaseConfigResourceType  = "GCP::Dataform::ReleaseConfig"
	WorkflowConfigResourceType = "GCP::Dataform::WorkflowConfig"
)

// workspaceOperations is StandardOperations without Update: the workspaces
// collection has no patch method at all, so every field is create-only.
// Declaring the operation and letting base answer NotUpdatable would be a
// worse-shaped lie - formae would plan an update that can only fail, where
// omitting it plans a replacement that works.
var workspaceOperations = []resource.Operation{
	resource.OperationCreate,
	resource.OperationRead,
	resource.OperationDelete,
	resource.OperationList,
	resource.OperationCheckStatus,
}

var dataformRegistry *base.ResourceRegistry

func init() {
	dataformRegistry = base.NewResourceRegistry(
		DataformAPI, DataformOperations, DataformNativeID)

	err := dataformRegistry.RegisterAll([]base.ResourceDefinition{
		{
			// The root of everything in Dataform: a container for SQL workflow
			// code, plus the settings that compilation and execution inherit.
			//
			// Free, and free without a Git remote. gitRemoteSettings is
			// genuinely optional - a repository created with none is a working,
			// unconnected repository - which matters because a remote needs a
			// personal access token in Secret Manager. Nothing is provisioned:
			// a repository holds metadata until a compilation result or a
			// workflow invocation runs BigQuery jobs, and neither of those is a
			// resource this plugin creates.
			//
			// A repository with workspaces, release configs or workflow configs
			// under it refuses to delete with 400 "has nested resources. If the
			// API supports cascading delete, set 'force' to true". No force is
			// sent: the nested types declare their repository, which makes the
			// repository the producer on that edge, and formae destroys the
			// consumer first. Cascading instead would delete children formae
			// does not manage.
			ResourceType: RepositoryResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "repositories",
				CreateIDParam:      "repositoryId", // id goes in ?repositoryId=, not the body
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  repositoryRequestTransformer,
			ResponseTransformer: repositoryResponseTransformer,
		},
		{
			// A development branch of a repository: an editable checkout where
			// code is written before it is committed. Free - it is metadata and
			// a file tree, and it compiles nothing on its own.
			//
			// No patch method, hence workspaceOperations. The only settable
			// field is disableMoves.
			ResourceType:        WorkspaceResourceType,
			ResourceConfig:      workspaceConfig,
			Operations:          workspaceOperations,
			RequestTransformer:  workspaceRequestTransformer,
			ResponseTransformer: workspaceResponseTransformer,
		},
		{
			// A named compilation target: which Git commitish to compile, and
			// what to override in the compilation settings. Free, and free by
			// more than convention here: a cronSchedule - the field that would
			// have the service compile on its own - is refused outright by this
			// project's configuration with 400 "Automatic release is not
			// supported in first-party repositories that enabled
			// strictActAsChecks". Without one, creating a release config
			// compiles nothing and costs nothing.
			//
			// gitCommitish is required but is not validated against the remote:
			// a repository with no Git remote at all accepts
			// gitCommitish = "main". So a release config is declarable on an
			// unconnected repository.
			ResourceType:        ReleaseConfigResourceType,
			ResourceConfig:      releaseConfigConfig,
			RequestTransformer:  releaseConfigRequestTransformer,
			ResponseTransformer: releaseConfigResponseTransformer,
		},
		{
			// The schedule half of Dataform: which release config to execute,
			// and when. Free only because "when" can be left out - a workflow
			// config with no cronSchedule creates no workflow invocations, and a
			// workflow invocation is what runs BigQuery jobs and bills.
			// Verified live: creating one with no schedule leaves
			// workflowInvocations and compilationResults both empty.
			//
			// invocationConfig.serviceAccount is effectively required, not
			// optional: a create without it answers 400 "Service account must be
			// set when strict act as checks are enabled".
			ResourceType:        WorkflowConfigResourceType,
			ResourceConfig:      workflowConfigConfig,
			RequestTransformer:  workflowConfigRequestTransformer,
			ResponseTransformer: workflowConfigResponseTransformer,
		},
	})
	if err != nil {
		panic(err)
	}
}

// nestedUnderRepository is the parent wiring the three nested collections
// share. The repository is a path component rather than a body field, and it is
// declared as a property so the reference orders both halves of the lifecycle:
// creates after the repository, destroys before it - which is what keeps the
// repository's own delete from being refused for having nested resources.
func nestedUnderRepository() *base.ParentResourceConfig {
	return &base.ParentResourceConfig{
		ParentType:     "repositories",
		PropertyName:   "repository",
		RequiresParent: true,
	}
}

var workspaceConfig = base.ResourceConfig{
	ResourceType:   "workspaces",
	CreateIDParam:  "workspaceId",
	ParentResource: nestedUnderRepository(),
	// No SupportsUpdate: the collection has no patch method.
}

var releaseConfigConfig = base.ResourceConfig{
	ResourceType:       "releaseConfigs",
	CreateIDParam:      "releaseConfigId",
	ParentResource:     nestedUnderRepository(),
	SupportsUpdate:     true,
	UpdateMaskFromBody: true,
}

var workflowConfigConfig = base.ResourceConfig{
	ResourceType:       "workflowConfigs",
	CreateIDParam:      "workflowConfigId",
	ParentResource:     nestedUnderRepository(),
	SupportsUpdate:     true,
	UpdateMaskFromBody: true,
}
