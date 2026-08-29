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

// The API wants sourceTable as a full path; a forma passes a resolvable that
// resolves to the bare table id, which is what gives formae the ordering edge.
func TestBackupRequestExpandsSourceTable(t *testing.T) {
	body, err := backupRequestTransformer(map[string]interface{}{
		"project":     "p",
		"instance":    "inst1",
		"cluster":     "cluster1",
		"name":        "bk1",
		"sourceTable": "tbl1",
		"expireTime":  "2026-09-05T00:00:00Z",
	}, base.TransformContext{Project: "p"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := body["sourceTable"], "projects/p/instances/inst1/tables/tbl1"; got != want {
		t.Errorf("sourceTable = %v, want %v", got, want)
	}
	for _, k := range []string{"project", "instance", "cluster"} {
		if _, ok := body[k]; ok {
			t.Errorf("%q addresses the backup in the URL and must not be a body field", k)
		}
	}
	if body["expireTime"] != "2026-09-05T00:00:00Z" {
		t.Errorf("expireTime must survive, got %v", body["expireTime"])
	}
}

// A full path written by hand must not be expanded twice.
func TestBackupRequestKeepsFullSourceTable(t *testing.T) {
	full := "projects/other/instances/i9/tables/t9"
	body, err := backupRequestTransformer(map[string]interface{}{
		"instance": "inst1", "sourceTable": full,
	}, base.TransformContext{Project: "p"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := body["sourceTable"]; got != full {
		t.Errorf("sourceTable = %v, want %v", got, full)
	}
}

// instance and cluster live only in the path, and the name comes back long.
func TestBackupResponseRecoversPathParts(t *testing.T) {
	out := backupResponseTransformer(map[string]interface{}{
		"name":        "projects/p/instances/inst1/clusters/cluster1/backups/bk1",
		"sourceTable": "projects/p/instances/inst1/tables/tbl1",
	}, base.TransformContext{Project: "p"})

	if out["name"] != "bk1" || out["instance"] != "inst1" || out["cluster"] != "cluster1" {
		t.Errorf("got %+v", out)
	}
	if out["sourceTable"] != "tbl1" {
		t.Errorf("sourceTable = %v, want tbl1", out["sourceTable"])
	}
}
