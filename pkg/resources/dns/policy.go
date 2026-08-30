// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dns

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// policyServerSet are reported by Cloud DNS and never sent back.
var policyServerSet = map[string]bool{
	"id":   true,
	"kind": true,
}

// policyRequestTransformer drops the project, which addresses the policy in the
// URL rather than describing it, and the fields only the server sets. The name
// stays: Cloud DNS takes it in the body rather than as a query parameter.
func policyRequestTransformer(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		if k == "project" || policyServerSet[k] {
			continue
		}
		body[k] = v
	}
	return body, nil
}

// policyResponseTransformer puts the project back, which the response does not
// carry, and drops the kind discriminator that is not a property of the policy.
func policyResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	out := make(map[string]interface{}, len(apiResponse)+1)
	for k, v := range apiResponse {
		if k == "kind" {
			continue
		}
		out[k] = v
	}
	if ctx.Project != "" {
		out["project"] = ctx.Project
	}
	return out
}
