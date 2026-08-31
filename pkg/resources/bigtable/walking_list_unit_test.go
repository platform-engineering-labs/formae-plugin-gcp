// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigtable

import "testing"

// The create id parameter is snake_case while the collection is camelCase.
// Trimming the plural alone produced "materializedView_id", which the API
// ignored before rejecting the create for an empty id.
func TestBigtableIDParam(t *testing.T) {
	for collection, want := range map[string]string{
		"instances":         "instance_id",
		"clusters":          "cluster_id",
		"tables":            "table_id",
		"backups":           "backup_id",
		"materializedViews": "materialized_view_id",
	} {
		if got := bigtableIDParam(collection); got != want {
			t.Errorf("bigtableIDParam(%q) = %q, want %q", collection, got, want)
		}
	}
}

// Backups and materialized views were registered in the registry but never
// bound to the provisioner that sends the id parameter, so both shipped broken.
func TestBackupAndViewCarryOperations(t *testing.T) {
	for _, rt := range []string{BackupResourceType, MaterializedViewResourceType} {
		def, ok := bigtableRegistry.Definitions[rt]
		if !ok {
			t.Fatalf("%s is not in the registry", rt)
		}
		if len(def.Operations) == 0 {
			t.Errorf("%s registered with no operations", rt)
		}
	}
}
