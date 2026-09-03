// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package binaryauthorization

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// platformPolicyResponseTransformer shortens the name and strips the same two
// server-owned fields. A platform policy has no nested output-only field: every
// check kind, down to the PEM inside a Sigstore authority, is echoed back byte
// for byte as sent (verified live against all seven check kinds).
func platformPolicyResponseTransformer(
	apiResponse map[string]interface{}, ctx base.TransformContext,
) map[string]interface{} {
	out := base.ShortNameResponseTransformer.Transform(apiResponse, ctx)
	dropServerOwnedFields(out)
	return out
}
