// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package compute

import (
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// wave builds the wave shape the API stores, optionally with the server's
// output-only ordinal on it.
func rolloutPlanWave(withNumber bool) map[string]interface{} {
	wave := map[string]interface{}{
		"displayName": "w1",
		"selectors": []interface{}{
			map[string]interface{}{
				"locationSelector": map[string]interface{}{
					// A bare zone name: the API refuses "zones/europe-central2-b".
					"includedLocations": []interface{}{"europe-central2-b"},
				},
			},
		},
		"validation": map[string]interface{}{
			"type": "time",
			"timeBasedValidationMetadata": map[string]interface{}{
				"waitDuration": "60s",
			},
		},
	}
	if withNumber {
		wave["number"] = "1"
	}
	return wave
}

// The server stamps `number` onto each stored wave. It is output-only and nested
// inside waves[], where hasProviderDefault cannot reach it - and because
// rolloutPlans has no update method, a read that disagrees with the declaration
// plans a *replacement*, not a patch. It must come off on read.
func TestRolloutPlanResponseStripsWaveNumber(t *testing.T) {
	out := rolloutPlanResponseTransformer(map[string]interface{}{
		"kind":              "compute#rolloutPlan",
		"id":                "123",
		"creationTimestamp": "2026-01-01T00:00:00Z",
		"selfLink":          "https://www.googleapis.com/compute/v1/projects/p/global/rolloutPlans/rp",
		"selfLinkWithId":    "https://www.googleapis.com/compute/v1/projects/p/global/rolloutPlans/123",
		"name":              "rp",
		"description":       "probe",
		"waves":             []interface{}{rolloutPlanWave(true)},
	}, base.TransformContext{Project: "p"})

	waves, ok := out["waves"].([]interface{})
	if !ok || len(waves) != 1 {
		t.Fatalf("waves not preserved as a one-entry list: %#v", out["waves"])
	}
	got, ok := waves[0].(map[string]interface{})
	if !ok {
		t.Fatalf("wave is not an object: %#v", waves[0])
	}
	if _, present := got["number"]; present {
		t.Errorf("waves[].number must be stripped: %#v", got)
	}

	// Everything a forma declares round-trips untouched.
	if !reflect.DeepEqual(got, rolloutPlanWave(false)) {
		t.Errorf("declared wave content altered:\n got %#v\nwant %#v", got, rolloutPlanWave(false))
	}

	// id and selfLink are what res.id / res.selfLink resolve against, so they
	// must survive; the rest of the top-level echo is not a schema field and is
	// left alone rather than stripped, matching every other compute type.
	for _, k := range []string{"name", "description", "id", "selfLink", "kind", "creationTimestamp"} {
		if _, present := out[k]; !present {
			t.Errorf("top-level %q must not be stripped", k)
		}
	}
}

// A read with no waves, or with something that is not a wave object, must pass
// through rather than panic - discovery reads whatever the API happens to hold.
func TestRolloutPlanResponseToleratesOddShapes(t *testing.T) {
	t.Run("no waves", func(t *testing.T) {
		out := rolloutPlanResponseTransformer(
			map[string]interface{}{"name": "rp"}, base.TransformContext{})
		if _, present := out["waves"]; present {
			t.Errorf("absent waves must not be invented: %#v", out)
		}
		if out["name"] != "rp" {
			t.Errorf("name altered: %#v", out)
		}
	})

	t.Run("non-object wave", func(t *testing.T) {
		out := rolloutPlanResponseTransformer(map[string]interface{}{
			"name":  "rp",
			"waves": []interface{}{"not-an-object"},
		}, base.TransformContext{})
		waves, _ := out["waves"].([]interface{})
		if len(waves) != 1 || waves[0] != "not-an-object" {
			t.Errorf("unexpected wave list: %#v", out["waves"])
		}
	})

	t.Run("waves not a list", func(t *testing.T) {
		out := rolloutPlanResponseTransformer(map[string]interface{}{
			"name":  "rp",
			"waves": "nonsense",
		}, base.TransformContext{})
		if out["waves"] != "nonsense" {
			t.Errorf("non-list waves altered: %#v", out["waves"])
		}
	})
}

// The transformer must not write through to the caller's map: base hands it the
// decoded response body, which is also what the operation poller looks at.
func TestRolloutPlanResponseDoesNotMutateInput(t *testing.T) {
	in := map[string]interface{}{
		"name":  "rp",
		"waves": []interface{}{rolloutPlanWave(true)},
	}
	rolloutPlanResponseTransformer(in, base.TransformContext{})

	wave, _ := in["waves"].([]interface{})[0].(map[string]interface{})
	if _, present := wave["number"]; !present {
		t.Errorf("input map was mutated: %#v", wave)
	}
}
