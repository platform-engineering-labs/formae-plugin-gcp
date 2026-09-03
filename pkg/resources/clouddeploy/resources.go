// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package clouddeploy

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

const (
	TargetResourceType           = "GCP::CloudDeploy::Target"
	DeliveryPipelineResourceType = "GCP::CloudDeploy::DeliveryPipeline"
	CustomTargetTypeResourceType = "GCP::CloudDeploy::CustomTargetType"
	DeployPolicyResourceType     = "GCP::CloudDeploy::DeployPolicy"
	AutomationResourceType       = "GCP::CloudDeploy::Automation"
)

var cloudDeployRegistry *base.ResourceRegistry

func init() {
	cloudDeployRegistry = base.NewResourceRegistry(
		CloudDeployAPI, CloudDeployOperations, CloudDeployNativeID)

	// Everything here is configuration: Cloud Deploy charges for the delivery
	// pipelines it *runs* (the Cloud Build execution a release renders and a
	// rollout deploys), not for the descriptions of them. The types below hold
	// no compute and start nothing on their own - a pipeline with no release
	// has nothing to promote, and an automation reacts to rollout events that
	// only a release produces.
	//
	// Deliberately absent, and not merely unimplemented: releases, rollouts and
	// jobRuns. A release triggers a Cloud Build render and a rollout performs an
	// actual deploy, so both cost money to create; neither is independently
	// declarable either, since a rollout is a record of an action taken rather
	// than a desired state. automationRuns are the same - a log of what an
	// automation did, with no create method at all.
	//
	// None carries a Scope: Cloud Deploy is regional throughout, and
	// ScopeLocationBased would make List return nothing whenever a target
	// declares only a region. See locationOf in api.go.
	err := cloudDeployRegistry.RegisterAll([]base.ResourceDefinition{
		{
			// Where a deploy lands: a Cloud Run location, a GKE or Anthos
			// cluster, several other targets at once, or a custom target type.
			//
			// A target is a *description* of a destination, not the destination
			// itself. It provisions nothing and is not validated against
			// anything: a target naming a GKE cluster that does not exist was
			// created live and read back happily. That is what makes it free to
			// hold, and it is also why a missing runtime shows up as a pipeline
			// condition rather than as a failed create.
			ResourceType: TargetResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "targets",
				CreateIDParam:      "targetId", // id goes in ?targetId=, not the body
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			// The custom target type reference is a full path on the wire and a
			// short name in a forma; see target.go for why both halves of that
			// translation have to exist.
			RequestTransformer:  base.RequestTransformerFunc(targetRequestTransformer),
			ResponseTransformer: targetResponseTransformer,
		},
		{
			// The promotion flow: an ordered list of stages, each naming a
			// target, that a release walks from first to last.
			//
			// A stage names its target by short id - the full path is rejected
			// as "not a valid resource ID for resource type stage.targetId" -
			// which is exactly the form a Target resolvable yields, so no
			// translation is needed on either side.
			//
			// Delete refuses while the pipeline still has nested resources
			// ("has nested resources. If the API supports cascading delete, set
			// 'force' to true"), and force is deliberately not sent: the nested
			// resources include the release and rollout history, which is not
			// something a reconcile should silently destroy. An Automation
			// declares its pipeline, which makes the pipeline the producer on a
			// default edge and has formae destroy the automation first.
			ResourceType: DeliveryPipelineResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "deliveryPipelines",
				CreateIDParam:      "deliveryPipelineId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			// The stages are echoed back exactly as sent, nested strategy and
			// deploy parameters included, so nothing but the name needs
			// translating.
			RequestTransformer:  base.DropFieldsOnUpdate("name"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// A deploy action Cloud Deploy does not know how to perform itself:
			// the render and deploy steps are supplied as Skaffold custom
			// actions or as container tasks, and a Target then names this type
			// instead of a runtime.
			//
			// Pure configuration - the container image named here is pulled by
			// a rollout, which this batch does not create, so nothing here
			// runs.
			ResourceType: CustomTargetTypeResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "customTargetTypes",
				CreateIDParam:      "customTargetTypeId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  base.DropFieldsOnUpdate("name"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// When deploys may not happen: a set of time windows, and the
			// pipelines and targets the restriction applies to. The policy
			// inhibits actions; it never performs any.
			ResourceType: DeployPolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "deployPolicies",
				CreateIDParam:      "deployPolicyId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  base.DropFieldsOnUpdate("name"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// Promote, advance or repair without a human: rules that fire on
			// rollout events, run as a named service account. Nested under the
			// pipeline whose releases it acts on, which is a path component
			// rather than a body field.
			//
			// An automation with no release to act on does nothing at all,
			// which is what makes it free to hold. The service account is
			// checked against IAM at create time - a non-existent one is
			// refused with "authorization check: generic::invalid_argument:
			// invalid service account" - so this is the one type here that
			// needs something outside Cloud Deploy to exist.
			ResourceType: AutomationResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "automations",
				CreateIDParam: "automationId",
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "deliveryPipelines",
					PropertyName:   "deliveryPipeline",
					RequiresParent: true,
				},
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  base.RequestTransformerFunc(automationRequestTransformer),
			ResponseTransformer: automationResponseTransformer,
		},
	})
	if err != nil {
		panic(err)
	}
}
