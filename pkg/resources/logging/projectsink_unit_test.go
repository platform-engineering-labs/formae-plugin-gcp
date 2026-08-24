// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package logging

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func sinkProps() map[string]interface{} {
	return map[string]interface{}{
		"name":           "my-sink",
		"destination":    "logging.googleapis.com/projects/p/locations/global/buckets/_Default",
		"filter":         `severity>="ERROR"`,
		"description":    "d",
		"writerIdentity": "serviceAccount:x@y.iam.gserviceaccount.com",
	}
}

// UpdateMaskFromBody derives the mask from the body's top-level fields, so an
// immutable field left in the body would be named in the mask and rejected.
func TestSinkUpdateBodyDropsImmutableFields(t *testing.T) {
	out, err := base.DropFieldsOnUpdate("name", "writerIdentity").Transform(sinkProps(), base.TransformContext{
		Operation: resource.OperationUpdate,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"name", "writerIdentity"} {
		if _, ok := out[k]; ok {
			t.Errorf("%q must not survive into an update body: %#v", k, out)
		}
	}
	for _, k := range []string{"destination", "filter", "description"} {
		if _, ok := out[k]; !ok {
			t.Errorf("mutable field %q must survive: %#v", k, out)
		}
	}
}

// Create carries the client-chosen sink id in "name", so the transformer must
// leave a create body alone.
func TestSinkUpdateBodyLeavesCreateAlone(t *testing.T) {
	out, err := base.DropFieldsOnUpdate("name", "writerIdentity").Transform(sinkProps(), base.TransformContext{
		Operation: resource.OperationCreate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["name"] != "my-sink" {
		t.Errorf("create body must keep name: %#v", out)
	}
}
