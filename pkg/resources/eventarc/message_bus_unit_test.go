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
	out := locationResponseTransformer("messageBuses")(map[string]interface{}{
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
	odd := locationResponseTransformer("messageBuses")(map[string]interface{}{"name": "bus-a"}, base.TransformContext{})
	if odd["name"] != "bus-a" {
		t.Errorf("short name mangled: %#v", odd)
	}
	if _, ok := odd["location"]; ok {
		t.Errorf("location invented: %#v", odd)
	}
	// A trigger's path must not be mistaken for a bus.
	trigger := locationResponseTransformer("messageBuses")(map[string]interface{}{
		"name": "projects/dev-1/locations/europe-west1/triggers/t-a",
	}, base.TransformContext{})
	if _, ok := trigger["location"]; ok {
		t.Errorf("a trigger path should not be parsed as a bus: %#v", trigger)
	}
}

// location addresses the URL; name stays so the engine can fill ?messageBusId=.
func TestMessageBusRequestTransformer(t *testing.T) {
	body, err := eventarcRequestTransformer(map[string]interface{}{
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

// The factory keys on the collection, so a pipeline's path parses for pipelines
// and not for buses.
func TestLocationResponseTransformerIsCollectionSpecific(t *testing.T) {
	path := "projects/dev-1/locations/europe-west1/pipelines/pl-a"
	asPipeline := locationResponseTransformer("pipelines")(
		map[string]interface{}{"name": path}, base.TransformContext{})
	if asPipeline["name"] != "pl-a" || asPipeline["location"] != "europe-west1" {
		t.Errorf("pipeline path should parse: %#v", asPipeline)
	}
	asBus := locationResponseTransformer("messageBuses")(
		map[string]interface{}{"name": path}, base.TransformContext{})
	if _, ok := asBus["location"]; ok {
		t.Errorf("pipeline path must not parse as a bus: %#v", asBus)
	}
}

// A forma passes bus.res.name so formae orders the creates; the plugin turns
// that short name into the path Eventarc wants. A path that is already full must
// be left exactly as it is.
func TestPipelineRequestTransformerExpandsMessageBus(t *testing.T) {
	ctx := base.TransformContext{Project: "dev-1", Location: "europe-west1"}
	body, err := pipelineRequestTransformer(map[string]interface{}{
		"name":     "pl-a",
		"location": "europe-west1",
		"destinations": []interface{}{
			map[string]interface{}{"messageBus": "bus-a"},
		},
	}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := body["location"]; ok {
		t.Errorf("location must not reach the body: %#v", body)
	}
	dest := body["destinations"].([]interface{})[0].(map[string]interface{})
	want := "projects/dev-1/locations/europe-west1/messageBuses/bus-a"
	if dest["messageBus"] != want {
		t.Errorf("expansion: %v", dest["messageBus"])
	}

	// An already-full path is untouched.
	body, _ = pipelineRequestTransformer(map[string]interface{}{
		"destinations": []interface{}{map[string]interface{}{"messageBus": want}},
	}, ctx)
	dest = body["destinations"].([]interface{})[0].(map[string]interface{})
	if dest["messageBus"] != want {
		t.Errorf("full path rewritten: %v", dest["messageBus"])
	}

	// Other destination kinds pass through, and the input is not mutated.
	props := map[string]interface{}{
		"destinations": []interface{}{
			map[string]interface{}{"httpEndpoint": map[string]interface{}{"uri": "https://example.com"}},
		},
	}
	body, _ = pipelineRequestTransformer(props, ctx)
	dest = body["destinations"].([]interface{})[0].(map[string]interface{})
	if _, ok := dest["httpEndpoint"]; !ok {
		t.Errorf("http destination lost: %#v", dest)
	}
	// The declared location is preferred over the target's.
	body, _ = pipelineRequestTransformer(map[string]interface{}{
		"location":     "us-central1",
		"destinations": []interface{}{map[string]interface{}{"messageBus": "bus-a"}},
	}, ctx)
	dest = body["destinations"].([]interface{})[0].(map[string]interface{})
	if dest["messageBus"] != "projects/dev-1/locations/us-central1/messageBuses/bus-a" {
		t.Errorf("declared location should win: %v", dest["messageBus"])
	}
}

// Expand on write, shorten on read. Asymmetry here is what made the first
// pipeline run report drift on all four comparison steps: the declared value
// resolves to the bus's short name, so the read has to report the short name too.
func TestPipelineTransformersRoundTrip(t *testing.T) {
	ctx := base.TransformContext{Project: "dev-1", Location: "europe-west1"}
	declared := map[string]interface{}{
		"name":         "pl-a",
		"location":     "europe-west1",
		"destinations": []interface{}{map[string]interface{}{"messageBus": "bus-a"}},
	}
	body, err := pipelineRequestTransformer(declared, ctx)
	if err != nil {
		t.Fatal(err)
	}
	// What the API would echo back for that request.
	apiResponse := map[string]interface{}{
		"name":         "projects/dev-1/locations/europe-west1/pipelines/pl-a",
		"destinations": body["destinations"],
	}
	out := pipelineResponseTransformer(apiResponse, ctx)
	dest := out["destinations"].([]interface{})[0].(map[string]interface{})
	if dest["messageBus"] != "bus-a" {
		t.Errorf("read should report the declared short name, got %v", dest["messageBus"])
	}
	if out["name"] != "pl-a" || out["location"] != "europe-west1" {
		t.Errorf("name/location: %#v", out)
	}

	// A destination with no bus is untouched.
	out = pipelineResponseTransformer(map[string]interface{}{
		"name":         "projects/dev-1/locations/europe-west1/pipelines/pl-a",
		"destinations": []interface{}{map[string]interface{}{"httpEndpoint": map[string]interface{}{"uri": "https://example.com"}}},
	}, ctx)
	dest = out["destinations"].([]interface{})[0].(map[string]interface{})
	if _, ok := dest["httpEndpoint"]; !ok {
		t.Errorf("http destination lost: %#v", dest)
	}
}
