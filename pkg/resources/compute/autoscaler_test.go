// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

const migURL = "https://www.googleapis.com/compute/v1/projects/p/zones/z/instanceGroupManagers/mig"

// The schema's instanceGroupManager must reach the API as "target"; sending the
// schema name would have GCP reject the autoscaler for a missing target.
func TestAutoscalerRequestRenamesTargetField(t *testing.T) {
	out, err := autoscalerRequestTransformer.Transform(map[string]interface{}{
		"name":                 "as",
		"instanceGroupManager": migURL,
		"autoscalingPolicy":    map[string]interface{}{"maxNumReplicas": 2},
	}, base.TransformContext{})
	if err != nil {
		t.Fatal(err)
	}
	if out["target"] != migURL {
		t.Errorf("target not set from instanceGroupManager: %#v", out)
	}
	if _, ok := out["instanceGroupManager"]; ok {
		t.Errorf("schema field must not survive into the request: %#v", out)
	}
	if out["name"] != "as" || out["autoscalingPolicy"] == nil {
		t.Errorf("other fields must survive: %#v", out)
	}
}

// Read must map "target" back and shorten the zone URL, or Verify compares the
// API's shape against the declared forma and reports drift.
func TestAutoscalerResponseRenamesBackAndShortensZone(t *testing.T) {
	out := autoscalerResponseTransformer.Transform(map[string]interface{}{
		"name":   "as",
		"target": migURL,
		"zone":   "https://www.googleapis.com/compute/v1/projects/p/zones/europe-central2-b",
	}, base.TransformContext{})
	if out["instanceGroupManager"] != migURL {
		t.Errorf("target not mapped back: %#v", out)
	}
	if _, ok := out["target"]; ok {
		t.Errorf("API field must be dropped: %#v", out)
	}
	if out["zone"] != "europe-central2-b" {
		t.Errorf("zone not shortened: %#v", out)
	}
}

// A response without the field must not gain an empty one - an invented
// instanceGroupManager would read as drift against the declared forma.
func TestAutoscalerResponseLeavesMissingTargetAlone(t *testing.T) {
	out := autoscalerResponseTransformer.Transform(map[string]interface{}{
		"name": "as",
	}, base.TransformContext{})
	if _, ok := out["instanceGroupManager"]; ok {
		t.Errorf("must not invent instanceGroupManager: %#v", out)
	}
}
