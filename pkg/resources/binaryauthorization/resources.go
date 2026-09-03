// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package binaryauthorization implements GCP Binary Authorization resources.
package binaryauthorization

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// The API collections, named once because both the path builder and the native
// ID parser branch on them.
const (
	attestorCollection       = "attestors"
	platformPolicyCollection = "policies"
)

const (
	AttestorResourceType       = "GCP::BinaryAuthorization::Attestor"
	PlatformPolicyResourceType = "GCP::BinaryAuthorization::PlatformPolicy"
)

var binaryAuthorizationRegistry *base.ResourceRegistry

func init() {
	binaryAuthorizationRegistry = base.NewResourceRegistry(
		BinaryAuthorizationAPI, BinaryAuthorizationOperations, BinaryAuthorizationNativeID)

	err := binaryAuthorizationRegistry.RegisterAll([]base.ResourceDefinition{
		{
			// A named public key, and the Grafeas note the signatures it
			// verifies are recorded against. An admission rule names attestors;
			// an attestor on its own admits nothing and costs nothing — it
			// holds a key, not an image.
			//
			// The note it points at lives in the Container Analysis API, which
			// this plugin does not ship and which need not even be enabled:
			// Binary Authorization does not resolve noteReference at create
			// time, so an attestor referring to a note that does not exist is
			// accepted (verified live, HTTP 200). That is what makes the type
			// declarable at all — the alternative would have been a
			// prerequisite in an API outside this plugin.
			//
			// Update is PUT, not PATCH: the API has no patch and no update
			// mask, and a PUT replaces the resource, so omitting `description`
			// really does clear it.
			ResourceType: AttestorResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   attestorCollection,
				Scope:          &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:  "attestorId", // id goes in ?attestorId=, not the body
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPut,
			},
			// name is the path. It has to survive a create — base reads the id
			// out of body["name"] for ?attestorId= and removes it itself — and
			// must leave an update body, where it is redundant at best.
			//
			// No update mask is involved, so this is not the usual
			// mask-pollution case; it is that a PUT body is the whole resource
			// and `name` is not part of it.
			RequestTransformer:  base.DropFieldsOnUpdate("name"),
			ResponseTransformer: base.ResponseTransformerFunc(attestorResponseTransformer),
		},
		{
			// The check-based successor to the project-wide policy: a named,
			// deletable document of image checks that a GKE cluster opts into.
			// Unlike projects/{p}/policy it is a real collection, so it has a
			// create and a delete and enforces nothing until a cluster names
			// it.
			//
			// Free, and inert on its own: the checks describe what would be
			// required of an image, and nothing evaluates them without a
			// cluster configured to use this policy.
			//
			// The platform is a URL segment rather than a field; see
			// gkePlatform in api.go for why it is a constant.
			//
			// ListItemsKey is set because the list response keys the array as
			// "platformPolicies" while the collection segment is "policies" —
			// base tries "items" then the resource type, so without this the
			// policies are never listed and never appear in inventory.
			ResourceType: PlatformPolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   platformPolicyCollection,
				Scope:          &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:  "policyId",
				ListItemsKey:   "platformPolicies",
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPut, // replacePlatformPolicy
			},
			RequestTransformer:  base.DropFieldsOnUpdate("name"),
			ResponseTransformer: base.ResponseTransformerFunc(platformPolicyResponseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}
}
