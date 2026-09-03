// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package parametermanager implements GCP Parameter Manager resources.
package parametermanager

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

const (
	ParameterResourceType        = "GCP::ParameterManager::Parameter"
	ParameterVersionResourceType = "GCP::ParameterManager::ParameterVersion"
)

var parameterManagerRegistry *base.ResourceRegistry

func init() {
	parameterManagerRegistry = base.NewResourceRegistry(
		ParameterManagerAPI, ParameterManagerOperations, ParameterManagerNativeID)

	err := parameterManagerRegistry.RegisterAll([]base.ResourceDefinition{
		{
			// A named container for configuration values, holding the format
			// its versions are written in and, optionally, the CMEK they are
			// encrypted with. The parameter itself holds no data - the versions
			// under it do - and it is free.
			//
			// Registered ScopeGlobal, which is also the only scope this plugin
			// offers for the type: a regional parameter is reachable only on
			// that region's own host and this one is only reachable on the
			// plain host. See globalLocation in api.go, and the 403 that looks
			// like an IAM error and is not.
			//
			// Worth knowing when writing a fixture: `labels` sent on create are
			// absent from the create response and present in the very next GET.
			// This API is synchronous, so the create response is what lands in
			// state and there is no operation to poll and no read-back behind
			// it - which is why the conformance case declares no labels and its
			// -update companion is what adds them. An update does read back.
			ResourceType: ParameterResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "parameters",
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "parameterId", // id goes in ?parameterId=, not the body
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			// name is the path, and format is fixed at creation - PATCH with
			// updateMask=format answers 400 IMMUTABLE_FIELD - so both have to
			// leave the body before the mask is built from it. A format change
			// is planned as a replace by the createOnly hint in the schema.
			//
			// kmsKey deliberately stays: the API documents it as a plain
			// optional field rather than an immutable one, so it should be
			// patchable. That is the one claim here not established live - a
			// CMEK probe needs a KMS key version, which bills monthly.
			RequestTransformer:  base.DropFieldsOnUpdate("name", "format"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// One immutable revision of a parameter's contents. This is the
			// resource that actually holds the data, and it is free.
			//
			// Nested under its parameter, which is a path component rather than
			// a body field. A parameter that still has versions refuses to
			// delete with HTTP 400 FAILED_PRECONDITION ("has nested
			// resources"), and this API's delete takes no force flag, so the
			// versions must go first. A version references its parameter, which
			// makes the parameter the producer on a default edge and formae
			// destroys the consumer first - so the declared reference is also
			// what gets the teardown order right.
			//
			// ListItemsKey is set because the collection segment ("versions")
			// is not the key the list response uses: it answers
			// {"parameterVersions": [...]}. base tries "items" and the resource
			// type before falling back to it, so naming the response key here
			// is the difference between a discoverable version and an invisible
			// one.
			//
			// The list response is the BASIC view - no payload - while a GET
			// defaults to FULL and returns it. See
			// dropParameterVersionPayload for why the read path removes it.
			ResourceType: ParameterVersionResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "versions",
				CreateIDParam: "parameterVersionId",
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "parameters",
					PropertyName:   "parameter",
					RequiresParent: true,
				},
				ListItemsKey:       "parameterVersions",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  parameterVersionRequestTransformer,
			ResponseTransformer: parameterVersionResponseTransformer,
		},
	})
	if err != nil {
		panic(err)
	}
}
