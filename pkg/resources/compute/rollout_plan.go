// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// rolloutPlanResponseTransformer normalizes a RolloutPlan read.
//
// The server stamps an ordinal onto every wave it stores - a plan created with
// one wave reads back with `"number": "1"` inside it. The field is output-only
// and appears nowhere in a forma, but it sits inside `waves[]`, where a schema
// hint cannot reach it (hasProviderDefault applies to top-level fields only), so
// it has to come off here. Without that, every read disagrees with the
// declaration - and because rolloutPlans has no update method at all, the
// disagreement plans a *replacement* rather than a no-op patch: the plan would
// be destroyed and recreated on every reconcile.
//
// Nothing else is stripped. kind, id, selfLink, selfLinkWithId and
// creationTimestamp are all provider-assigned top-level fields that no forma
// declares, and no compute type in this package strips them: they are not schema
// fields, so they are ignored on comparison, and id and selfLink are exactly
// what `res.id` / `res.selfLink` resolve against - removing them would break
// every reference to the plan.
func rolloutPlanResponseTransformer(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
	result := make(map[string]interface{}, len(apiResponse))
	for k, v := range apiResponse {
		result[k] = v
	}

	waves, ok := result["waves"].([]interface{})
	if !ok {
		return result
	}

	cleaned := make([]interface{}, len(waves))
	for i, raw := range waves {
		wave, ok := raw.(map[string]interface{})
		if !ok {
			cleaned[i] = raw
			continue
		}
		copied := make(map[string]interface{}, len(wave))
		for k, v := range wave {
			if k == "number" {
				continue
			}
			copied[k] = v
		}
		cleaned[i] = copied
	}
	result["waves"] = cleaned

	return result
}

// RolloutPlanResponseTransformer is the response transformer for RolloutPlan.
var RolloutPlanResponseTransformer = base.ResponseTransformerFunc(rolloutPlanResponseTransformer)
