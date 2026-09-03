// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dataform

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// repositoryRequestTransformer drops what a PATCH may not carry.
//
// "name" survives a create - base reads the repository id out of it for
// ?repositoryId= and removes it itself - and has to leave an update, because
// the updateMask is built from the body's top-level fields and a name in the
// mask is a name change.
//
// "kmsKeyName" is immutable, and the refusal is by mask rather than by value:
// PATCH ?updateMask=kmsKeyName answers 400 "Request update_mask contains
// immutable fields: [kms_key_name]" even when the value is unchanged. A
// reconcile PATCH sends every declared field, so the key would enter the mask
// on every update of an unrelated field and every one of those updates would
// fail.
var repositoryRequestTransformer = base.DropFieldsOnUpdate("name", "kmsKeyName")

// repositoryResponseTransformer drops the fields GCP populates unbidden and
// shortens the full-path name to the id a forma declared.
//
// Top-level, in the order the API reports them:
//
//   - createTime, internalMetadata - server bookkeeping.
//   - teamFolderName - always reported, and as the empty string for a
//     repository that belongs to no team folder. Team folders are a separate
//     collection this plugin does not implement.
//   - dataEncryptionState - present only on a KMS-protected repository, and it
//     names a key *version*, which rotates. Nested, so a schema hint could not
//     reach it even if it were declarable.
//   - containingFolder - deliberately not in the schema. It is immutable via
//     PATCH (400 "immutable fields: [containing_folder]") and can only be moved
//     by the repositories.move custom verb, which is not CRUD, so a declared
//     value formae could never reconcile would be worse than none.
//
// Inside gitRemoteSettings, two more that hasProviderDefault cannot reach
// because they are nested:
//
//   - effectiveDefaultBranch - the remote's branch, echoed back as "main" for a
//     repository that never sent a defaultBranch. Verified live: a PATCH
//     setting url + defaultBranch answers with all three.
//   - tokenStatus - documented in the API's own discovery as deprecated and
//     carrying no token status information.
var repositoryResponseTransformer = &base.CompositeResponseTransformer{
	Transformers: []base.ResponseTransformer{
		base.ResponseTransformerFunc(
			func(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
				if git, ok := apiResponse["gitRemoteSettings"].(map[string]interface{}); ok {
					delete(git, "effectiveDefaultBranch")
					delete(git, "tokenStatus")
				}
				return apiResponse
			}),
		dropFields(
			"createTime",
			internalMetadata,
			"teamFolderName",
			"dataEncryptionState",
			"containingFolder",
		),
		base.ShortNameResponseTransformer,
	},
}
