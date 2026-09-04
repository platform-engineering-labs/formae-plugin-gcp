// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package networksecurity

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// dropEmptyStrings returns a ResponseTransformer that removes the named fields
// when the API reports them as the empty string.
//
// Several Network Security responses materialise an optional string reference
// that was never sent as "" rather than leaving it out. A field that is
// declared in the schema but omitted in a forma then arrives in state carrying
// a value the forma never wrote, and every sync reads that as drift on a
// resource nobody touched.
//
// hasProviderDefault is the other way to tolerate this, and it is the right
// answer when the provider's value means something (an assigned etag, a
// defaulted enum). Here it means nothing at all - "no policy attached" - so the
// field is dropped instead of being carried around as a phantom empty
// reference.
func dropEmptyStrings(fields ...string) base.ResponseTransformer {
	return base.ResponseTransformerFunc(
		func(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
			for _, f := range fields {
				if v, ok := apiResponse[f].(string); ok && v == "" {
					delete(apiResponse, f)
				}
			}
			return apiResponse
		})
}

// gatewaySecurityPolicyResponseTransformer strips the empty tlsInspectionPolicy
// GCP invents for a policy created without one, then shortens the full-path
// name to the id the forma declared.
var gatewaySecurityPolicyResponseTransformer = &base.CompositeResponseTransformer{
	Transformers: []base.ResponseTransformer{
		dropEmptyStrings("tlsInspectionPolicy"),
		base.ShortNameResponseTransformer,
	},
}
