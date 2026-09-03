//go:build unit

// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package clouddeploy

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// The pipeline addresses the automation rather than describing it: it is not a
// field of the Automation message and must never reach the body, on create or
// on update.
func TestAutomationRequestDropsPipeline(t *testing.T) {
	for _, op := range []resource.Operation{resource.OperationCreate, resource.OperationUpdate} {
		out, err := automationRequestTransformer(map[string]interface{}{
			"name":             "auto1",
			"deliveryPipeline": "dp1",
			"serviceAccount":   "sa@example.iam.gserviceaccount.com",
		}, base.TransformContext{Project: "p1", Location: "europe-central2", Operation: op})
		if err != nil {
			t.Fatalf("transform: %v", err)
		}
		if _, ok := out["deliveryPipeline"]; ok {
			t.Errorf("deliveryPipeline should be dropped for %v", op)
		}
		if _, ok := out["name"]; ok != (op == resource.OperationCreate) {
			t.Errorf("name presence wrong for %v: %v", op, ok)
		}
	}
}

// Cloud Deploy adds a condition inside every rule it returns. It sits two
// levels below a declared field, where no hasProviderDefault can reach, so it
// has to be stripped here or Verify rejects the resource.
func TestAutomationResponseStripsRuleConditions(t *testing.T) {
	out := automationResponseTransformer.Transform(map[string]interface{}{
		"name": "projects/p1/locations/europe-central2/deliveryPipelines/dp1/automations/auto1",
		"rules": []interface{}{
			map[string]interface{}{
				"promoteReleaseRule": map[string]interface{}{
					"id":        "promote",
					"wait":      "3600s",
					"condition": map[string]interface{}{"targetsPresentCondition": map[string]interface{}{}},
				},
			},
			map[string]interface{}{
				"advanceRolloutRule": map[string]interface{}{
					"id":        "advance",
					"condition": map[string]interface{}{"targetsPresentCondition": map[string]interface{}{}},
				},
			},
		},
	}, base.TransformContext{Project: "p1", Location: "europe-central2"})

	rules := out["rules"].([]interface{})
	promote := rules[0].(map[string]interface{})["promoteReleaseRule"].(map[string]interface{})
	if _, ok := promote["condition"]; ok {
		t.Error("promoteReleaseRule.condition should be stripped")
	}
	if promote["wait"] != "3600s" {
		t.Error("promoteReleaseRule.wait should survive")
	}
	advance := rules[1].(map[string]interface{})["advanceRolloutRule"].(map[string]interface{})
	if _, ok := advance["condition"]; ok {
		t.Error("advanceRolloutRule.condition should be stripped")
	}
}

// The pipeline lives only in the path, so the response has to put it back
// before the name is shortened - otherwise the declared property is absent from
// state and every sync reads it as drift.
func TestAutomationResponseRestoresPipeline(t *testing.T) {
	out := automationResponseTransformer.Transform(map[string]interface{}{
		"name": "projects/p1/locations/europe-central2/deliveryPipelines/dp1/automations/auto1",
	}, base.TransformContext{Project: "p1", Location: "europe-central2"})

	if out["deliveryPipeline"] != "dp1" {
		t.Errorf("deliveryPipeline = %v, want dp1", out["deliveryPipeline"])
	}
	if out["name"] != "auto1" {
		t.Errorf("name = %v, want auto1", out["name"])
	}
}
