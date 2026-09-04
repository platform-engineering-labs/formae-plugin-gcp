// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package networksecurity

import (
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// gatewaySecurityPolicyRuleRequestTransformer drops the two properties that
// address the rule rather than describe it.
//
// "gatewaySecurityPolicy" is a path component: it goes unconditionally, because
// the API rejects it as an unknown body field on create as well as on update.
// "name" is dropped on update only - create reads the rule id out of it for
// ?gatewaySecurityPolicyRuleId= - and it must go there because the updateMask
// is built from the body's top-level fields, so a name left in would land in
// the mask and the patch would be refused.
var gatewaySecurityPolicyRuleRequestTransformer = &base.CompositeRequestTransformer{
	Transformers: []base.RequestTransformer{
		base.DropFields("gatewaySecurityPolicy"),
		base.DropFieldsOnUpdate("name"),
	},
}

// parentPolicyOf lifts the owning policy out of a rule's full resource path,
// projects/{p}/locations/{loc}/gatewaySecurityPolicies/{policy}/rules/{rule}.
// It returns "" for anything else, so a response that is not shaped like a rule
// leaves the property alone rather than gaining a wrong one.
func parentPolicyOf(name string) string {
	parts := strings.Split(name, "/")
	if len(parts) == 8 && parts[4] == "gatewaySecurityPolicies" && parts[6] == "rules" {
		return parts[5]
	}
	return ""
}

// gatewaySecurityPolicyRuleResponseTransformer puts back what the path carries
// and drops what the API invents.
//
// The owning policy is recovered from the reported name before the name is
// shortened - it is a path component, never a body field, so without this the
// property a forma declared is simply missing from state and reads as drift.
// Recovering it from the response rather than from the transform context is
// what makes discovery work too: a List goes through the "-" wildcard parent,
// so the context has no parent to offer, but every listed item still reports
// its own full path.
//
// applicationMatcher comes back as "" for a rule created without one; see
// dropEmptyStrings. tlsInspectionEnabled is left alone: it is a top-level bool,
// so a hasProviderDefault hint reaches it in the schema.
var gatewaySecurityPolicyRuleResponseTransformer = &base.CompositeResponseTransformer{
	Transformers: []base.ResponseTransformer{
		base.ResponseTransformerFunc(
			func(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
				if name, ok := apiResponse["name"].(string); ok {
					if policy := parentPolicyOf(name); policy != "" {
						apiResponse["gatewaySecurityPolicy"] = policy
					}
				}
				return apiResponse
			}),
		dropEmptyStrings("applicationMatcher"),
		base.ShortNameResponseTransformer,
	},
}
