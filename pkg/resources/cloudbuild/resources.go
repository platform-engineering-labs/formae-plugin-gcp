// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package cloudbuild implements GCP Cloud Build resources.
package cloudbuild

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

const (
	BuildTriggerResourceType = "GCP::CloudBuild::BuildTrigger"
)

var cloudBuildRegistry *base.ResourceRegistry

func init() {
	cloudBuildRegistry = base.NewResourceRegistry(
		CloudBuildAPI, CloudBuildOperations, CloudBuildNativeID)

	err := cloudBuildRegistry.RegisterAll([]base.ResourceDefinition{
		{
			// The configuration that says when a build runs and what it runs.
			// A trigger is not a build: creating one provisions nothing and
			// queues nothing. That was verified rather than reasoned about -
			// the project's builds collection was empty before the first probe
			// trigger was created and still empty after it, and after six more.
			// A build only starts when something fires the trigger, which for
			// the shapes this type supports means an explicit triggers.run or
			// an incoming webhook, neither of which formae issues.
			//
			// Cloud Build's trigger methods are synchronous, alone among the
			// mutating calls in this API, hence the OperationConfig override -
			// see CloudBuildSyncOperations for what the async path would do
			// instead of degrading.
			//
			// UpdateMaskFromBody is off on purpose: a maskless PATCH is a full
			// update of the mutable fields, which is what a reconcile wants.
			// With a mask built from the body, a field dropped from the forma
			// would leave the mask and silently keep its old value.
			//
			// No CreateIDParam: the id travels in the body as "name". Cloud
			// Build has no ?triggerId= parameter, and a create with no name at
			// all does not fail - it produces a trigger literally named
			// "trigger", which is how a missing id would leak.
			ResourceType:    BuildTriggerResourceType,
			OperationConfig: CloudBuildSyncOperations,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "triggers",
				SupportsUpdate:     true,
				UpdateMaskFromBody: false,
			},
			RequestTransformer:  base.RequestTransformerFunc(buildTriggerRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(buildTriggerResponseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}
}
