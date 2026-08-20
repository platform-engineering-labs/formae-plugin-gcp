// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package monitoring

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// The schema's "name" carries the metric type; the API field is "type", which
// formae's base Resource class reserves.
func TestMetricDescriptorNameBecomesType(t *testing.T) {
	out, err := metricDescriptorNameToType.Transform(map[string]interface{}{
		"name":       "custom.googleapis.com/formae/latency",
		"metricKind": "GAUGE",
	}, base.TransformContext{})
	if err != nil {
		t.Fatal(err)
	}
	if out["type"] != "custom.googleapis.com/formae/latency" {
		t.Errorf("type not set from name: %#v", out)
	}
	if _, ok := out["name"]; ok {
		t.Errorf("name must not reach the body: %#v", out)
	}
	if out["metricKind"] != "GAUGE" {
		t.Errorf("other fields must survive: %#v", out)
	}
}

// Read must fold the type back into "name" and drop the full-path name the API
// returns, or the identifier would not round-trip.
func TestMetricDescriptorTypeBecomesName(t *testing.T) {
	out := metricDescriptorTypeToName(map[string]interface{}{
		"name": "projects/p/metricDescriptors/custom.googleapis.com/formae/latency",
		"type": "custom.googleapis.com/formae/latency",
	}, base.TransformContext{})
	if out["name"] != "custom.googleapis.com/formae/latency" {
		t.Errorf("name not folded from type: %#v", out["name"])
	}
	if _, ok := out["type"]; ok {
		t.Errorf("API type field must be dropped: %#v", out)
	}
}

// The id contains slashes, so the native ID cannot be split pairwise.
func TestParseMetricDescriptorNativeID(t *testing.T) {
	ctx, err := parseMetricDescriptorNativeID(
		"projects/dev-1/metricDescriptors/custom.googleapis.com/formae/latency")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Project != "dev-1" {
		t.Errorf("project: %q", ctx.Project)
	}
	if ctx.ResourceType != "metricDescriptors" {
		t.Errorf("resourceType: %q", ctx.ResourceType)
	}
	if ctx.ResourceName != "custom.googleapis.com/formae/latency" {
		t.Errorf("resourceName lost its slashes: %q", ctx.ResourceName)
	}
}

func TestParseMetricDescriptorNativeIDRejectsGarbage(t *testing.T) {
	for _, bad := range []string{
		"custom.googleapis.com/formae/latency",
		"projects/p/metricDescriptors/",
		"projects//metricDescriptors/x",
	} {
		if _, err := parseMetricDescriptorNativeID(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}
