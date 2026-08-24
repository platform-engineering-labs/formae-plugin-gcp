// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import "testing"

const attachmentPath = "projects/dev-1/zones/europe-central2-b/disks/d1/resourcePolicies/nightly"

// The attachment has no identity beyond the (disk, policy) pair, so every part
// must survive the round-trip.
func TestAttachmentNativeIDRoundTrip(t *testing.T) {
	if got := buildAttachmentNativeID("dev-1", "europe-central2-b", "d1", "nightly"); got != attachmentPath {
		t.Fatalf("build: %q", got)
	}
	project, zone, disk, policy, err := parseAttachmentNativeID(attachmentPath)
	if err != nil {
		t.Fatal(err)
	}
	if project != "dev-1" || zone != "europe-central2-b" || disk != "d1" || policy != "nightly" {
		t.Errorf("parse: %q %q %q %q", project, zone, disk, policy)
	}
}

func TestParseAttachmentNativeIDRejectsOtherShapes(t *testing.T) {
	for _, bad := range []string{
		"projects/dev-1/zones/europe-central2-b/disks/d1",                // the disk itself
		"projects/dev-1/regions/r/disks/d1/resourcePolicies/nightly",     // regional shape
		"projects/dev-1/global/networks/n/peerings/p",                    // a peering id
		"projects/dev-1/zones/z/disks/d1/resourcePolicies/nightly/extra", // too long
		"",
	} {
		if _, _, _, _, err := parseAttachmentNativeID(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

// The verbs want a full regional policy path; the region is implied by the
// disk's zone, so a caller can give just the policy name.
func TestPolicyURLDerivesRegionFromZone(t *testing.T) {
	got := policyURLFor("dev-1", "europe-central2-b", "nightly")
	want := "projects/dev-1/regions/europe-central2/resourcePolicies/nightly"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// A caller who already wrote a path must not have it rewritten.
func TestPolicyURLLeavesPathsAlone(t *testing.T) {
	full := "projects/other/regions/us-central1/resourcePolicies/weekly"
	if got := policyURLFor("dev-1", "europe-central2-b", full); got != full {
		t.Errorf("rewrote a full path: %q", got)
	}
}

// Read compares by bare name, so a URL from the disk's list must reduce cleanly.
func TestPolicyNameOf(t *testing.T) {
	cases := map[string]string{
		"https://www.googleapis.com/compute/v1/projects/p/regions/r/resourcePolicies/nightly": "nightly",
		"projects/p/regions/r/resourcePolicies/nightly":                                       "nightly",
		"nightly": "nightly",
	}
	for in, want := range cases {
		if got := policyNameOf(in); got != want {
			t.Errorf("policyNameOf(%q) = %q want %q", in, got, want)
		}
	}
}
