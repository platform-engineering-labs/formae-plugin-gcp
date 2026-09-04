// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dataform

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// workspaceRequestTransformer drops the repository, which addresses the
// workspace rather than describing it. The API answers a body carrying it with
// 400 "Unknown name \"repository\" at 'workspace': Cannot find field", so it
// goes unconditionally rather than on update only.
//
// "name" is left alone: base reads the workspace id out of it for
// ?workspaceId= and removes it itself, and there is no update path to protect -
// the collection has no patch method at all.
var workspaceRequestTransformer = base.DropFields("repository")

// workspaceResponseTransformer puts the repository back and drops what the API
// invents.
//
//   - createTime, internalMetadata - server bookkeeping.
//   - privateResourceMetadata - a nested object holding one output-only bool
//     that the API's own discovery document says is always true for a
//     workspace. Nested, so a schema hint could not reach it.
//   - dataEncryptionState - present only on a KMS-protected workspace, and it
//     names a key version, which rotates. Nested for the same reason.
//
// disableMoves is deliberately *not* dropped: it is a top-level bool that comes
// back as false when it was never sent, which a hasProviderDefault hint in the
// schema covers, and unlike the fields above it is genuinely settable.
var workspaceResponseTransformer = &base.CompositeResponseTransformer{
	Transformers: []base.ResponseTransformer{
		restoreRepository,
		dropFields(
			"createTime",
			internalMetadata,
			"privateResourceMetadata",
			"dataEncryptionState",
		),
		base.ShortNameResponseTransformer,
	},
}
