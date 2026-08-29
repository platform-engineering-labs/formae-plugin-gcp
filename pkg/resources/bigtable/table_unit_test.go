// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package bigtable

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// The API reports a table's name as a full path; a forma declares the short id
// and the instance separately. Without shortening, a table reports drift the
// moment it is created.
func TestTableResponseTransformerShortensName(t *testing.T) {
	out := tableResponseTransformer(map[string]interface{}{
		"name": "projects/p/instances/inst1/tables/tbl1",
	}, base.TransformContext{Project: "p"})

	if got, want := out["name"], "tbl1"; got != want {
		t.Errorf("name = %v, want %v", got, want)
	}
	if got, want := out["instance"], "inst1"; got != want {
		t.Errorf("instance = %v, want %v", got, want)
	}
	if got, want := out["project"], "p"; got != want {
		t.Errorf("project = %v, want %v", got, want)
	}
}

// An instance's own path must not be read as a table's.
func TestTableResponseTransformerIgnoresOtherShapes(t *testing.T) {
	out := tableResponseTransformer(map[string]interface{}{
		"name": "projects/p/instances/inst1",
	}, base.TransformContext{Project: "p"})
	if got, want := out["name"], "projects/p/instances/inst1"; got != want {
		t.Errorf("name = %v, want %v", got, want)
	}
	if _, ok := out["instance"]; ok {
		t.Error("instance must not be inferred from a non-table path")
	}
}
