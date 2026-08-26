// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dataproc

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const AutoscalingPolicyResourceType = "GCP::Dataproc::AutoscalingPolicy"
const SessionTemplateResourceType = "GCP::Dataproc::SessionTemplate"
const WorkflowTemplateResourceType = "GCP::Dataproc::WorkflowTemplate"

var dataprocRegistry *base.ResourceRegistry

func init() {
	dataprocRegistry = base.NewResourceRegistry(DataprocAPI, DataprocOperations, DataprocNativeID)

	// AutoscalingPolicy is region-scoped and synchronous. It does NOT accept the
	// id as a query parameter (unlike Artifact Registry's repositoryId), so
	// CreateIDParam is not used. Instead the API takes the id in the request
	// body as "id"; the user declares the short identifier as "name", and
	// dataprocIDRequestTransformer copies name->id (dropping the output-only
	// "name") before the create body is sent. ShortNameResponseTransformer maps
	// the API's full-path "name" back to the short id the user declared.
	err := dataprocRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: AutoscalingPolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "autoscalingPolicies",
				Scope:          &base.ScopeConfig{Type: base.ScopeRegional},
				SupportsUpdate: false,      // update is a PUT; defer until verified
				ListItemsKey:   "policies", // list response is {"policies":[...]}, not "items"
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			RequestTransformer:  dataprocIDRequestTransformer,
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// Serverless Spark session template. Location-scoped, so it carries
			// its own APIConfig and native-ID parser, and synchronous like the
			// autoscaling policy above.
			ResourceType:   SessionTemplateResourceType,
			APIConfig:      DataprocLocationAPI,
			NativeIDConfig: DataprocLocationNativeID,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "sessionTemplates",
				Scope:          &base.ScopeConfig{Type: base.ScopeLocationBased},
				SupportsUpdate: true,
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationUpdate,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			RequestTransformer:  base.RequestTransformerFunc(sessionTemplateRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(sessionTemplateResponseTransformer),
		},
		{
			// A Spark job graph plus the cluster to run it on. Region-scoped like
			// the autoscaling policy, so it shares the default API config.
			// Update is a PUT carrying the current version, which the engine
			// supplies through optimistic locking - the version is a number, not
			// a string etag.
			ResourceType: WorkflowTemplateResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "workflowTemplates",
				Scope:        &base.ScopeConfig{Type: base.ScopeRegional},
				// list response is {"templates":[...]}, which matches neither
				// "items" nor the collection name, so nothing was ever discovered.
				ListItemsKey:   "templates",
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPut,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       true,
					FieldName:     "version",
					LocationInURL: false,
				},
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationUpdate,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			ResponseTransformer: base.ResponseTransformerFunc(workflowTemplateResponseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}
}
