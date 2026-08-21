// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package eventarc

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// A bus declares its own location because Eventarc Advanced is regional-subset,
// so the read has to report it back or the declared field looks missing.
func TestMessageBusResponseTransformerRecoversLocation(t *testing.T) {
	out := messageBusResponseTransformer(map[string]interface{}{
		"name":          "projects/dev-1/locations/europe-west1/messageBuses/bus-a",
		"uid":           "abc",
		"loggingConfig": map[string]interface{}{"logSeverity": "NONE"},
	}, base.TransformContext{})

	if out["name"] != "bus-a" {
		t.Errorf("name should shorten: %v", out["name"])
	}
	if out["location"] != "europe-west1" {
		t.Errorf("location not recovered: %#v", out)
	}
	if out["uid"] != "abc" {
		t.Errorf("uid must survive: %#v", out)
	}
	if _, ok := out["loggingConfig"]; !ok {
		t.Errorf("loggingConfig must survive: %#v", out)
	}

	// An unexpected shape passes through rather than half-parsing.
	odd := messageBusResponseTransformer(map[string]interface{}{"name": "bus-a"}, base.TransformContext{})
	if odd["name"] != "bus-a" {
		t.Errorf("short name mangled: %#v", odd)
	}
	if _, ok := odd["location"]; ok {
		t.Errorf("location invented: %#v", odd)
	}
	// A trigger's path must not be mistaken for a bus.
	trigger := messageBusResponseTransformer(map[string]interface{}{
		"name": "projects/dev-1/locations/europe-west1/triggers/t-a",
	}, base.TransformContext{})
	if _, ok := trigger["location"]; ok {
		t.Errorf("a trigger path should not be parsed as a bus: %#v", trigger)
	}
}

// location addresses the URL; name stays so the engine can fill ?messageBusId=.
func TestMessageBusRequestTransformer(t *testing.T) {
	body, err := messageBusRequestTransformer(map[string]interface{}{
		"name":     "bus-a",
		"location": "europe-west1",
		"labels":   map[string]interface{}{"env": "test"},
	}, base.TransformContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := body["location"]; ok {
		t.Errorf("location must not reach the body: %#v", body)
	}
	if body["name"] != "bus-a" {
		t.Errorf("name must stay for the create id: %#v", body)
	}
	if _, ok := body["labels"]; !ok {
		t.Errorf("labels must survive: %#v", body)
	}
}
