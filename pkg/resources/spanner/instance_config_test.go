// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package spanner

import (
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// The create envelope carries the id twice, in two different shapes: once as
// "instanceConfigId" beside the wrapped object, and once as the full path
// inside it. Omitting the inner name was rejected 400 "Invalid
// CreateInstanceConfig request." with a fieldViolation on instance_config.name.
func TestInstanceConfigCreateEnvelope(t *testing.T) {
	body, err := instanceConfigRequestTransformer(map[string]interface{}{
		"name":        "custom-my-config",
		"displayName": "Formae conformance",
		"baseConfig":  "eur6",
		"replicas": []interface{}{
			map[string]interface{}{"location": "europe-west4", "type": "READ_WRITE"},
		},
		// Output only - the API owns these and must never see them back.
		"configType": "USER_MANAGED",
		"etag":       "abc",
		"state":      "READY",
	}, base.TransformContext{Project: "p", Operation: resource.OperationCreate})
	if err != nil {
		t.Fatalf("transform: %v", err)
	}

	if body["instanceConfigId"] != "custom-my-config" {
		t.Errorf("instanceConfigId = %v", body["instanceConfigId"])
	}
	if _, present := body["updateMask"]; present {
		t.Errorf("create must not carry an update mask: %+v", body)
	}

	cfg, ok := body["instanceConfig"].(map[string]interface{})
	if !ok {
		t.Fatalf("instanceConfig = %T", body["instanceConfig"])
	}
	if cfg["name"] != "projects/p/instanceConfigs/custom-my-config" {
		t.Errorf("instanceConfig.name = %v", cfg["name"])
	}
	// A forma names the base config by its bare id; the API wants a full path.
	if cfg["baseConfig"] != "projects/p/instanceConfigs/eur6" {
		t.Errorf("baseConfig = %v", cfg["baseConfig"])
	}
	for _, k := range []string{"configType", "etag", "state"} {
		if _, present := cfg[k]; present {
			t.Errorf("output-only %q reached the request body: %+v", k, cfg)
		}
	}
}

func TestInstanceConfigCreateNeedsAName(t *testing.T) {
	if _, err := instanceConfigRequestTransformer(map[string]interface{}{
		"displayName": "Formae",
	}, base.TransformContext{Project: "p", Operation: resource.OperationCreate}); err == nil {
		t.Error("a nameless instance config was accepted")
	}
}

// The patch envelope puts the mask in the BODY beside the wrapped object, not
// in the query string, and sends only the two fields Spanner documents as
// updatable plus the name that addresses the configuration.
func TestInstanceConfigUpdateEnvelope(t *testing.T) {
	body, err := instanceConfigRequestTransformer(map[string]interface{}{
		"name":        "custom-my-config",
		"displayName": "Formae conformance 2",
		"labels":      map[string]interface{}{"k": "v"},
		// Immutable: neither may appear in the patch body or its mask.
		"baseConfig": "eur6",
		"replicas": []interface{}{
			map[string]interface{}{"location": "europe-west4", "type": "READ_WRITE"},
		},
		"etag": "abc",
	}, base.TransformContext{Project: "p", Operation: resource.OperationUpdate})
	if err != nil {
		t.Fatalf("transform: %v", err)
	}

	want := map[string]interface{}{
		"updateMask": "displayName,labels",
		"instanceConfig": map[string]interface{}{
			"name":        "projects/p/instanceConfigs/custom-my-config",
			"displayName": "Formae conformance 2",
			"labels":      map[string]interface{}{"k": "v"},
		},
	}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("got  %#v\nwant %#v", body, want)
	}
}

// The mask is fixed rather than derived from the fields present, so that a
// forma which drops its labels actually clears them. A mask computed from the
// body would omit "labels" exactly when the clearing is wanted, making the
// removal a silent no-op.
func TestInstanceConfigUpdateMaskIsFixedSoLabelsCanBeCleared(t *testing.T) {
	body, err := instanceConfigRequestTransformer(map[string]interface{}{
		"name":        "custom-my-config",
		"displayName": "Formae conformance",
	}, base.TransformContext{Project: "p", Operation: resource.OperationUpdate})
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	if body["updateMask"] != "displayName,labels" {
		t.Errorf("updateMask = %v", body["updateMask"])
	}
	cfg := body["instanceConfig"].(map[string]interface{})
	if _, present := cfg["labels"]; present {
		t.Errorf("labels were invented: %+v", cfg)
	}
}

// baseConfig is immutable, so expanding on the request without shortening on
// the response would leave the declared value and the stored state
// permanently disagreeing - and every re-apply would then plan a replacement
// the API refuses. The two halves must be exact inverses.
func TestBaseConfigRoundTrips(t *testing.T) {
	for _, short := range []string{"eur6", "nam3", "regional-europe-west4"} {
		full, ok := qualifyBaseConfig(short, "proj-1").(string)
		if !ok {
			t.Fatalf("qualifyBaseConfig(%q) did not return a string", short)
		}
		out := instanceConfigResponseTransformer(
			map[string]interface{}{"baseConfig": full}, base.TransformContext{})
		if out["baseConfig"] != short {
			t.Errorf("round trip of %q via %q gave %v", short, full, out["baseConfig"])
		}
	}
}

// A value that is already a path is passed through, so a forma may name a base
// config in another project explicitly.
func TestQualifyBaseConfigLeavesAPathAlone(t *testing.T) {
	full := "projects/other/instanceConfigs/nam3"
	if got := qualifyBaseConfig(full, "p"); got != full {
		t.Errorf("baseConfig = %v", got)
	}
}

// optionalReplicas is every replica location GCP offers for the base config -
// a very large array, never user-declarable. Keeping it would bloat the state
// of every configuration with a catalogue that belongs to Google.
func TestInstanceConfigResponseStripsOutputOnlyNoise(t *testing.T) {
	out := instanceConfigResponseTransformer(map[string]interface{}{
		"name":        "projects/p/instanceConfigs/custom-my-config",
		"displayName": "Formae conformance",
		"baseConfig":  "projects/p/instanceConfigs/eur6",
		"configType":  "USER_MANAGED",
		"etag":        "f3jiHNZkRysK0Vjxm0rAkgFt7vTgZpUd1SRPlhyWf+w=",
		"state":       "READY",
		"optionalReplicas": []interface{}{
			map[string]interface{}{"location": "us-east1", "type": "READ_ONLY"},
		},
		"reconciling":                   false,
		"leaderOptions":                 []interface{}{"europe-west4"},
		"freeInstanceAvailability":      "UNAVAILABLE",
		"quorumType":                    "DUAL_REGION",
		"storageLimitPerProcessingUnit": "1024",
	}, base.TransformContext{})

	want := map[string]interface{}{
		// The "custom-" prefix is part of the id Spanner requires, not a
		// namespace the plugin adds, so it survives the shortening.
		"name":        "custom-my-config",
		"displayName": "Formae conformance",
		"baseConfig":  "eur6",
		"configType":  "USER_MANAGED",
		"etag":        "f3jiHNZkRysK0Vjxm0rAkgFt7vTgZpUd1SRPlhyWf+w=",
		"state":       "READY",
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("got  %#v\nwant %#v", out, want)
	}
}

// The API reports defaultLeaderLocation only where it is true. A nested field
// cannot be marked hasProviderDefault, so an echoed `false` would be a field
// no forma declared - drop it and keep the two shapes identical.
func TestInstanceConfigResponseNormalizesReplicas(t *testing.T) {
	out := instanceConfigResponseTransformer(map[string]interface{}{
		"name": "projects/p/instanceConfigs/custom-my-config",
		"replicas": []interface{}{
			map[string]interface{}{
				"location": "europe-west4", "type": "READ_WRITE", "defaultLeaderLocation": true,
			},
			map[string]interface{}{
				"location": "europe-west3", "type": "READ_WRITE", "defaultLeaderLocation": false,
			},
			map[string]interface{}{"location": "us-east1", "type": "READ_ONLY"},
		},
	}, base.TransformContext{})

	want := []interface{}{
		map[string]interface{}{
			"location": "europe-west4", "type": "READ_WRITE", "defaultLeaderLocation": true,
		},
		map[string]interface{}{"location": "europe-west3", "type": "READ_WRITE"},
		map[string]interface{}{"location": "us-east1", "type": "READ_ONLY"},
	}
	if !reflect.DeepEqual(out["replicas"], want) {
		t.Errorf("got  %#v\nwant %#v", out["replicas"], want)
	}
}

// A short name has already been transformed (a create's own path context), so
// the response transformer must be idempotent rather than mangling it.
func TestInstanceConfigResponseLeavesShortNamesAlone(t *testing.T) {
	out := instanceConfigResponseTransformer(map[string]interface{}{
		"name":       "custom-my-config",
		"baseConfig": "eur6",
	}, base.TransformContext{})
	if out["name"] != "custom-my-config" || out["baseConfig"] != "eur6" {
		t.Errorf("got %#v", out)
	}
}

// An instance configuration is a sibling collection of instances, not a child
// of one, so the native ID parser must accept it without expecting
// "/instances/" and must reject anything deeper.
func TestParseNativeIDHandlesInstanceConfigs(t *testing.T) {
	ctx, err := parseSpannerNativeID("projects/p/instanceConfigs/custom-my-config")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := base.PathContext{
		Project: "p", ResourceType: "instanceConfigs", ResourceName: "custom-my-config",
	}
	if !reflect.DeepEqual(ctx, want) {
		t.Errorf("got  %#v\nwant %#v", ctx, want)
	}

	for _, bad := range []string{
		"projects/p/instanceConfigs/custom-my-config/replicas/0",
		"projects/p/instanceConfigs",
	} {
		if _, err := parseSpannerNativeID(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestInstanceConfigIsRegistered(t *testing.T) {
	for _, op := range []resource.Operation{
		resource.OperationCreate, resource.OperationRead, resource.OperationUpdate,
		resource.OperationDelete, resource.OperationList,
	} {
		if !registry.HasProvisioner(InstanceConfigResourceType, op) {
			t.Errorf("%s not registered for %v", InstanceConfigResourceType, op)
		}
	}
}
