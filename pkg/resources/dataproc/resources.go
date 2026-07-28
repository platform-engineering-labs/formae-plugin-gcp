// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dataproc

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const AutoscalingPolicyResourceType = "GCP::Dataproc::AutoscalingPolicy"

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
				SupportsUpdate: false, // update is a PUT; defer until verified
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
	})
	if err != nil {
		panic(err)
	}
}
