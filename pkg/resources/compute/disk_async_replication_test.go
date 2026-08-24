// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import "testing"

const asyncReplicationPath = "projects/dev-1/zones/europe-central2-a/disks/pri/asyncReplication/europe-west1-b/sec"

func TestAsyncReplicationNativeIDRoundTrip(t *testing.T) {
	got := buildAsyncReplicationNativeID("dev-1", "europe-central2-a", "pri", "europe-west1-b", "sec")
	if got != asyncReplicationPath {
		t.Fatalf("build: %q", got)
	}
	project, pz, pri, sz, sec, err := parseAsyncReplicationNativeID(asyncReplicationPath)
	if err != nil {
		t.Fatal(err)
	}
	if project != "dev-1" || pz != "europe-central2-a" || pri != "pri" ||
		sz != "europe-west1-b" || sec != "sec" {
		t.Errorf("parse: %q %q %q %q %q", project, pz, pri, sz, sec)
	}
	for _, bad := range []string{
		"projects/dev-1/zones/europe-central2-a/disks/pri",                        // just the disk
		"projects/dev-1/zones/europe-central2-a/disks/pri/asyncReplication/sec",   // no secondary zone
		"projects/dev-1/zones/europe-central2-a/disks/pri/asyncReplication//sec",  // empty zone
		"projects/dev-1/regions/europe-central2/disks/pri/asyncReplication/z/sec", // regional disk
		"",
	} {
		if _, _, _, _, _, err := parseAsyncReplicationNativeID(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestDiskRefParts(t *testing.T) {
	cases := map[string][2]string{
		"https://www.googleapis.com/compute/v1/projects/dev-1/zones/europe-west1-b/disks/sec": {"europe-west1-b", "sec"},
		"projects/dev-1/zones/europe-central2-a/disks/pri":                                    {"europe-central2-a", "pri"},
		"zones/us-central1-f/disks/d":                                                         {"us-central1-f", "d"},
	}
	for ref, want := range cases {
		zone, name, ok := diskRefParts(ref)
		if !ok || zone != want[0] || name != want[1] {
			t.Errorf("%q -> %q %q %v", ref, zone, name, ok)
		}
	}
	for _, bad := range []string{
		"projects/dev-1/regions/europe-west1/disks/d", // regional: no zone
		"projects/dev-1/zones/europe-west1-b",         // no disk
		"",
	} {
		if _, _, ok := diskRefParts(bad); ok {
			t.Errorf("expected failure for %q", bad)
		}
	}
}

// Stopping replication leaves asyncPrimaryDisk in place and only flips the
// state, so a read that keyed on the field would report a dead pair as live
// forever. This is the single most important behaviour of this resource.
func TestAsyncReplicationStateDistinguishesStopped(t *testing.T) {
	active := map[string]interface{}{
		"asyncPrimaryDisk": map[string]interface{}{"disk": "projects/dev-1/zones/europe-central2-a/disks/pri"},
		"resourceStatus": map[string]interface{}{
			"asyncPrimaryDisk": map[string]interface{}{"state": "ACTIVE"},
		},
	}
	stopped := map[string]interface{}{
		"asyncPrimaryDisk": map[string]interface{}{"disk": "projects/dev-1/zones/europe-central2-a/disks/pri"},
		"resourceStatus": map[string]interface{}{
			"asyncPrimaryDisk": map[string]interface{}{"state": "STOPPED"},
		},
	}
	if asyncReplicationState(active) != asyncReplicationActiveState {
		t.Error("active pair not reported active")
	}
	if asyncReplicationState(stopped) == asyncReplicationActiveState {
		t.Error("stopped pair reported active - the field survives the stop, only state changes")
	}
	// Both still name the primary, which is exactly why state is the deciding
	// factor rather than presence.
	if asyncPrimaryDiskRef(stopped) == "" {
		t.Error("stopped pair should still name its primary")
	}

	// A disk that never replicated, and junk shapes, must be quiet.
	for _, empty := range []map[string]interface{}{
		{"name": "plain-disk"},
		{"resourceStatus": map[string]interface{}{}},
		{"resourceStatus": "not-an-object"},
		{"asyncPrimaryDisk": "not-an-object"},
	} {
		if asyncReplicationState(empty) != "" {
			t.Errorf("unexpected state for %#v", empty)
		}
		if asyncPrimaryDiskRef(empty) != "" {
			t.Errorf("unexpected primary for %#v", empty)
		}
	}
}
