// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package eventarc implements GCP Eventarc resources.
package eventarc

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const TriggerResourceType = "GCP::Eventarc::Trigger"
const MessageBusResourceType = "GCP::Eventarc::MessageBus"

var eventarcRegistry *base.ResourceRegistry

func init() {
	eventarcRegistry = base.NewResourceRegistry(
		EventarcAPI, EventarcOperations, EventarcNativeID)

	// ponytail: Update deferred (as artifactregistry/dns/cloudrun do) until PATCH
	// is verified live. create/delete are the async LRO paths this proves out.
	err := eventarcRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: TriggerResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "triggers",
				Scope:         &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam: "triggerId", // id goes in ?triggerId=, not the body
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
			// The Eventarc Advanced hub that pipelines and enrollments attach
			// to. Supports PATCH, unlike Trigger above.
			ResourceType: MessageBusResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "messageBuses",
				Scope:              &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam:      "messageBusId", // id goes in ?messageBusId=
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
			RequestTransformer:  base.RequestTransformerFunc(messageBusRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(messageBusResponseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}
}
