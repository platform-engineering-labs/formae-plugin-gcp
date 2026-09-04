// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package clouddeploy

import (
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// automationRuleKinds are the four one-of members of an AutomationRule. Each is
// a message with an "id" and, unbidden, a "condition".
var automationRuleKinds = []string{
	"promoteReleaseRule",
	"advanceRolloutRule",
	"repairRolloutRule",
	"timedPromoteReleaseRule",
}

// automationRequestTransformer drops the pipeline (a path component) and the
// name on update.
//
// "deliveryPipeline" addresses the automation rather than describing it, so it
// goes unconditionally - it is not a field of the Automation message at all.
// "name" is dropped on update only: create reads the id (?automationId=) out of
// it, and on a patch it would land in the body-derived update mask, where the
// API documents name as output-only.
func automationRequestTransformer(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	return (&base.CompositeRequestTransformer{Transformers: []base.RequestTransformer{
		base.DropFields("deliveryPipeline"),
		base.DropFieldsOnUpdate("name"),
	}}).Transform(props, ctx)
}

// automationResponseTransformer strips the condition Cloud Deploy invents
// inside every rule, lifts the owning pipeline back out of the path, then
// shortens the full-path name.
var automationResponseTransformer = &base.CompositeResponseTransformer{
	Transformers: []base.ResponseTransformer{
		base.ResponseTransformerFunc(
			func(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
				stripRuleConditions(apiResponse)

				// The pipeline is a path component, not a body field, so
				// recover it from the name before ShortNameResponseTransformer
				// discards the path:
				// projects/{p}/locations/{l}/deliveryPipelines/{dp}/automations/{a}
				if name, ok := apiResponse["name"].(string); ok {
					parts := strings.Split(name, "/")
					if len(parts) == 8 && parts[4] == "deliveryPipelines" && parts[6] == "automations" {
						apiResponse["deliveryPipeline"] = parts[5]
					}
				}
				return apiResponse
			}),
		base.ShortNameResponseTransformer,
	},
}

// stripRuleConditions removes the output-only "condition" block Cloud Deploy
// adds to each automation rule.
//
// A rule sent as {"promoteReleaseRule": {"id": "promote", "wait": "3600s"}}
// comes back as {"promoteReleaseRule": {"id": "promote", "wait": "3600s",
// "condition": {"targetsPresentCondition": {}}}} - observed live on create and
// on patch. It reports whether the targets the rule would promote to exist,
// which is a fact about the world rather than anything a forma declares.
//
// It cannot be tolerated with hasProviderDefault either: a schema hint reaches
// only a top-level field, and this one is two levels down inside the declared
// "rules" list, so Verify would see a property that is neither declared nor
// defaulted and fail the case. Dropping it is the only place this can be fixed.
func stripRuleConditions(apiResponse map[string]interface{}) {
	rules, ok := apiResponse["rules"].([]interface{})
	if !ok {
		return
	}
	for _, raw := range rules {
		rule, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		for _, kind := range automationRuleKinds {
			if body, ok := rule[kind].(map[string]interface{}); ok {
				delete(body, "condition")
			}
		}
	}
}
