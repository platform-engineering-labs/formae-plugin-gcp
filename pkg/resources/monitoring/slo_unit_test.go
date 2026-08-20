// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package monitoring

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// "service" is a URL path component; Monitoring rejects it as an unknown body
// field, which is what made the first SLO create attempt fail.
var sloBody = &base.CompositeRequestTransformer{
	Transformers: []base.RequestTransformer{
		base.DropFields("service"),
		base.DropFieldsOnUpdate("name"),
	},
}

func TestSloBodyStripsServiceOnCreate(t *testing.T) {
	out, err := sloBody.Transform(map[string]interface{}{
		"name":        "my-slo",
		"service":     "my-svc",
		"goal":        0.99,
		"displayName": "d",
	}, base.TransformContext{Operation: resource.OperationCreate})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out["service"]; ok {
		t.Errorf("service must never reach the body: %#v", out)
	}
	// On create the id still travels via CreateIDParam, which reads "name".
	if out["name"] != "my-slo" {
		t.Errorf("create body should still carry name: %#v", out)
	}
	if out["goal"] != 0.99 {
		t.Errorf("goal must survive: %#v", out)
	}
}

// On update "name" would land in the updateMask, so it goes too.
func TestSloBodyStripsNameAndServiceOnUpdate(t *testing.T) {
	out, err := sloBody.Transform(map[string]interface{}{
		"name":        "my-slo",
		"service":     "my-svc",
		"displayName": "d",
	}, base.TransformContext{Operation: resource.OperationUpdate})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"name", "service"} {
		if _, ok := out[k]; ok {
			t.Errorf("%q must not reach an update body: %#v", k, out)
		}
	}
	if out["displayName"] != "d" {
		t.Errorf("displayName must survive: %#v", out)
	}
}

// The API never returns "service"; without lifting it out of the resource name
// the stored state would not match the declared forma.
func TestSloServiceLiftedFromName(t *testing.T) {
	out := sloServiceFromName.Transform(map[string]interface{}{
		"name": "projects/989754770009/services/my-svc/serviceLevelObjectives/my-slo",
	}, base.TransformContext{})
	if out["service"] != "my-svc" {
		t.Errorf("service not lifted from name: %#v", out)
	}
}

// A response without a parseable name must not gain a bogus service field.
func TestSloServiceAbsentWhenNameUnparseable(t *testing.T) {
	out := sloServiceFromName.Transform(map[string]interface{}{
		"name": "projects/p/somethingElse/x",
	}, base.TransformContext{})
	if _, ok := out["service"]; ok {
		t.Errorf("must not invent a service: %#v", out)
	}
}

// A custom service needs the empty "custom" marker; formae prunes empty
// sub-objects, so the provisioner injects it.
func TestCustomServiceMarkerInjectedOnCreate(t *testing.T) {
	out, err := customServiceMarker.Transform(map[string]interface{}{
		"displayName": "svc",
	}, base.TransformContext{Operation: resource.OperationCreate})
	if err != nil {
		t.Fatal(err)
	}
	m, ok := out["custom"].(map[string]interface{})
	if !ok || len(m) != 0 {
		t.Errorf("custom marker not injected as an empty object: %#v", out)
	}
}
