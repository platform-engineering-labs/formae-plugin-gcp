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
// carry, and drops the kind discriminator.
//
// Cloud DNS stamps "kind" on nested objects as well as the policy itself - every
// entry of networks and of targetNameServers carries one - so it has to be
// stripped all the way down. Left in, it appears as a property the forma never
// declared and that nothing marks as a provider default.
func policyResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	out, _ := stripKind(apiResponse).(map[string]interface{})
	if out == nil {
		out = map[string]interface{}{}
	}
	if ctx.Project != "" {
		out["project"] = ctx.Project
	}
	return out
}

// stripKind removes every "kind" discriminator from a decoded JSON value.
func stripKind(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for k, item := range v {
			if k == "kind" {
				continue
			}
			out[k] = stripKind(item)
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(v))
		for _, item := range v {
			out = append(out, stripKind(item))
		}
		return out
	default:
		return value
	}
}
