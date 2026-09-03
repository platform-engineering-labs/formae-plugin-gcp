// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package binaryauthorization

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// serverOwnedFields are the top-level fields Binary Authorization sets for
// itself and reports back unbidden.
//
// Neither is declared in the schema, so a read that carried them would present
// Verify with fields the forma has no opinion about. hasProviderDefault is the
// other answer and would keep them in state, but neither is worth keeping:
// `updateTime` changes on every write, and `etag` is a concurrency token this
// plugin never replays — it is not accepted as a query parameter here and
// putting it in an update body would stake a claim on a version the server has
// already moved past.
//
// `etag` is also the reason this list exists at all rather than being folded
// into ShortNameResponseTransformer: attestors.create does NOT return it and
// attestors.get does, so the same resource read two ways would otherwise carry
// two different field sets.
var serverOwnedFields = []string{"etag", "updateTime"}

// dropServerOwnedFields removes the fields above from a response.
func dropServerOwnedFields(apiResponse map[string]interface{}) {
	for _, f := range serverOwnedFields {
		delete(apiResponse, f)
	}
}

// attestorResponseTransformer shortens the full-path name to the declared id
// and strips the two server-owned top-level fields plus the one nested
// output-only field.
//
// delegationServiceAccountEmail is the nested one, and it is the reason this is
// a transformer rather than a schema hint: it appears inside
// userOwnedGrafeasNote on every read (the per-project service agent Binary
// Authorization queries Container Analysis as), hasProviderDefault reaches only
// top-level fields, and Verify compares a nested object as a whole. Left in
// place it reads as drift against a forma that only ever wrote noteReference
// and publicKeys.
func attestorResponseTransformer(
	apiResponse map[string]interface{}, ctx base.TransformContext,
) map[string]interface{} {
	out := base.ShortNameResponseTransformer.Transform(apiResponse, ctx)
	dropServerOwnedFields(out)
	if note, ok := out["userOwnedGrafeasNote"].(map[string]interface{}); ok {
		delete(note, "delegationServiceAccountEmail")
	}
	return out
}
