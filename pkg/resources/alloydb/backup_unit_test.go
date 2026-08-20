// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package alloydb

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

var backupCtx = base.TransformContext{Project: "dev-1", Location: "europe-central2"}

const clusterPath = "projects/dev-1/locations/europe-central2/clusters/c1"

// The schema exposes a short cluster id so a forma can reference it through a
// resolvable; the API wants the full path under a different field name.
func TestBackupRequestExpandsCluster(t *testing.T) {
	out, err := backupRequestTransformer.Transform(map[string]interface{}{
		"cluster": "c1",
		"type":    "ON_DEMAND",
	}, backupCtx)
	if err != nil {
		t.Fatal(err)
	}
	if out["clusterName"] != clusterPath {
		t.Errorf("clusterName not expanded: %#v", out["clusterName"])
	}
	if _, ok := out["cluster"]; ok {
		t.Errorf("schema field must not reach the body: %#v", out)
	}
	if out["type"] != "ON_DEMAND" {
		t.Errorf("other fields must survive: %#v", out)
	}
}

// A caller who already wrote the full path must not have it mangled.
func TestBackupRequestLeavesFullPathAlone(t *testing.T) {
	out, err := backupRequestTransformer.Transform(map[string]interface{}{
		"cluster": clusterPath,
	}, backupCtx)
	if err != nil {
		t.Fatal(err)
	}
	if out["clusterName"] != clusterPath {
		t.Errorf("full path rewritten: %#v", out["clusterName"])
	}
}

// With no cluster there is nothing to expand, and the transformer must not
// invent an empty clusterName - the API would reject it.
func TestBackupRequestWithoutClusterAddsNothing(t *testing.T) {
	out, err := backupRequestTransformer.Transform(map[string]interface{}{
		"type": "ON_DEMAND",
	}, backupCtx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out["clusterName"]; ok {
		t.Errorf("must not invent clusterName: %#v", out)
	}
}

// Read must fold the path back to the short id, or Verify reports drift against
// the id the forma declared.
func TestBackupResponseFoldsClusterName(t *testing.T) {
	out := backupResponseTransformer.Transform(map[string]interface{}{
		"name":        "projects/dev-1/locations/europe-central2/backups/b1",
		"clusterName": clusterPath,
	}, backupCtx)
	if out["cluster"] != "c1" {
		t.Errorf("cluster not folded: %#v", out["cluster"])
	}
	if _, ok := out["clusterName"]; ok {
		t.Errorf("API field must be dropped: %#v", out)
	}
	if out["name"] != "b1" {
		t.Errorf("name not shortened: %#v", out["name"])
	}
}
