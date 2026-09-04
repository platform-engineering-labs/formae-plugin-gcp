// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dataform

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// releaseConfigRequestTransformer drops the repository from every body and the
// two create-only fields from update bodies.
//
//   - "repository" is a path component. The API answers a body carrying it with
//     400 "Unknown name \"repository\" at 'release_config': Cannot find field".
//   - "name" survives a create - base reads the id out of it for
//     ?releaseConfigId= - and has to leave an update so it does not enter the
//     mask as a rename.
//   - "codeCompilationConfig" is immutable, and the refusal is by mask rather
//     than by value: PATCH ?updateMask=codeCompilationConfig answers 400
//     "Request update_mask contains immutable fields:
//     [code_compilation_config]" even when the object is unchanged. Since a
//     reconcile PATCH sends every declared field, leaving it in would fail
//     every update of gitCommitish, timeZone or disabled.
var releaseConfigRequestTransformer = &base.CompositeRequestTransformer{
	Transformers: []base.RequestTransformer{
		base.DropFields("repository"),
		base.DropFieldsOnUpdate("name", "codeCompilationConfig"),
	},
}

// releaseConfigResponseTransformer puts the repository back and drops what the
// API owns.
//
//   - internalMetadata - server bookkeeping, and for a release config it also
//     carries the Dataform service agent's name.
//   - recentScheduledReleaseRecords - the last ten scheduled compilation
//     attempts. Output-only and it grows on its own.
//   - releaseCompilationResult - written by the *service*, not the caller: the
//     API sets it whenever automatic release creates a compilation result. It
//     is nominally an input field, but a forma cannot own it (declaring it
//     would mean pinning a compilation result formae did not create), and it
//     appears from nowhere on any repository where automatic release runs.
var releaseConfigResponseTransformer = &base.CompositeResponseTransformer{
	Transformers: []base.ResponseTransformer{
		restoreRepository,
		dropFields(
			internalMetadata,
			"recentScheduledReleaseRecords",
			"releaseCompilationResult",
		),
		base.ShortNameResponseTransformer,
	},
}
