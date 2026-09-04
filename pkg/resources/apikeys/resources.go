// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package apikeys implements GCP API Keys resources.
package apikeys

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// keyCollection is the API collection segment, named once because the path
// builder and the native ID parser both use it.
const keyCollection = "keys"

const KeyResourceType = "GCP::ApiKeys::Key"

var apiKeysRegistry *base.ResourceRegistry

func init() {
	apiKeysRegistry = base.NewResourceRegistry(ApiKeysAPI, ApiKeysOperations, ApiKeysNativeID)

	err := apiKeysRegistry.RegisterAll([]base.ResourceDefinition{
		{
			// An API key: a credential a client sends as ?key=... to identify
			// the calling project, plus the restrictions that narrow where it
			// may be used from and which services it may reach.
			//
			// Free — a key is a credential record, and it provisions nothing.
			// It is not, however, harmless: the key string is a bearer
			// credential, which is why keyResponseTransformer refuses to store
			// it and why a leaked key matters more than a leaked test bucket.
			//
			// Two API behaviours shape this registration:
			//
			// The id is client-chosen. `keyId` is documented as optional and a
			// key created without one is named by a server-assigned uuid, which
			// would make the resource unnameable and — worse — invisible to a
			// prefix sweep. Passing it through CreateIDParam means the resource
			// is addressed as "formae-test-..." like everything else here. The
			// id must match [a-z]([a-z0-9-]{0,61}[a-z0-9])? and, the docs are
			// explicit, must NOT itself look like a uuid.
			//
			// A deleted id stays reserved for thirty days. Delete is a soft
			// delete, and re-creating the same keyId inside the retention
			// window is refused: the create answers HTTP 200 and the operation
			// then completes with code 6, FLOW_APIKEY_ALREADY_EXISTS, "A key
			// with this id already exists." So a replace of an existing key
			// under the same name cannot succeed, which is why the conformance
			// fixture for this type exercises update rather than replace —
			// displayName, restrictions and annotations are all mutable, so
			// there is nothing a replace would have to prove.
			ResourceType: KeyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  keyCollection,
				Scope:         &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam: "keyId", // id goes in ?keyId=, not the body
				// The list response keys the array as "keys", which happens to
				// equal the collection segment base falls back to, so
				// ListItemsKey is not needed.
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
				// A soft-deleted key keeps reading back HTTP 200 for thirty
				// days; see keyDeleted.
				ReadTreatAsMissing: keyDeleted,
			},
			// name is the path, and the update mask is built from the body:
			// PATCH ?updateMask=name is rejected outright (HTTP 400) because a
			// key cannot be renamed. serviceAccountEmail is refused the same
			// way, by name — "Update mask contains immutable field(s)
			// \"service_account_email\"", APIKEYS_UPDATE_MASK_CONTAINS_IMMUTABLE_FIELDS
			// — so it too has to leave an update body while surviving a create,
			// where it is what binds the key to a service account.
			RequestTransformer:  base.DropFieldsOnUpdate("name", "serviceAccountEmail"),
			ResponseTransformer: base.ResponseTransformerFunc(keyResponseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}
}
