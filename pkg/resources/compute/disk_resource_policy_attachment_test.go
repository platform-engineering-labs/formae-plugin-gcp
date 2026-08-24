// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

const attachmentPath = "projects/dev-1/zones/europe-central2-b/disks/d1/resourcePolicies/nightly"

// The attachment has no identity beyond the (disk, policy) pair, so every part
// must survive the round-trip.
func TestAttachmentNativeIDRoundTrip(t *testing.T) {
	if got := zonalAttachmentProvisioner().buildAttachmentNativeID("dev-1", "europe-central2-b", "d1", "nightly"); got != attachmentPath {
		t.Fatalf("build: %q", got)
	}
	project, zone, disk, policy, err := zonalAttachmentProvisioner().parseAttachmentNativeID(attachmentPath)
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
		if _, _, _, _, err := zonalAttachmentProvisioner().parseAttachmentNativeID(bad); err == nil {
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

// zonalAttachmentProvisioner is the kind the tests above assert on; the two
// kinds share one implementation, so the flag is what distinguishes them.
func zonalAttachmentProvisioner() *DiskResourcePolicyAttachmentProvisioner {
	return &DiskResourcePolicyAttachmentProvisioner{BaseResource: &base.BaseResource{APIConfig: ComputeAPI}}
}

func regionalAttachmentProvisioner() *DiskResourcePolicyAttachmentProvisioner {
	return &DiskResourcePolicyAttachmentProvisioner{
		BaseResource: &base.BaseResource{APIConfig: ComputeAPI},
		regional:     true,
	}
}

// A regional disk lives under regions/{region}, so the two id shapes must not
// cross over — a regional attachment read against a zonal URL would 404.
func TestRegionalAttachmentNativeID(t *testing.T) {
	const path = "projects/dev-1/regions/europe-central2/disks/d-1/resourcePolicies/pol-a"
	r := regionalAttachmentProvisioner()
	if got := r.buildAttachmentNativeID("dev-1", "europe-central2", "d-1", "pol-a"); got != path {
		t.Fatalf("build: %q", got)
	}
	project, location, disk, policy, err := r.parseAttachmentNativeID(path)
	if err != nil {
		t.Fatal(err)
	}
	if project != "dev-1" || location != "europe-central2" || disk != "d-1" || policy != "pol-a" {
		t.Errorf("parse: %q %q %q %q", project, location, disk, policy)
	}
	if _, _, _, _, err := r.parseAttachmentNativeID(
		"projects/dev-1/zones/europe-central2-a/disks/d-1/resourcePolicies/pol-a"); err == nil {
		t.Error("regional kind accepted a zonal ID")
	}
	if _, _, _, _, err := zonalAttachmentProvisioner().parseAttachmentNativeID(path); err == nil {
		t.Error("zonal kind accepted a regional ID")
	}
}

// The policy must resolve into the disk's region either way: a zone needs its
// trailing letter dropped, a region is already right.
func TestRegionOfZone(t *testing.T) {
	for in, want := range map[string]string{
		"europe-central2-a": "europe-central2",
		"us-central1-f":     "us-central1",
		"europe-central2":   "europe-central2",
		"us-central1":       "us-central1",
	} {
		if got := regionOfZone(in); got != want {
			t.Errorf("regionOfZone(%q) = %q, want %q", in, got, want)
		}
	}
	if got := policyURLFor("dev-1", "europe-central2", "pol-a"); got != "projects/dev-1/regions/europe-central2/resourcePolicies/pol-a" {
		t.Errorf("regional policy URL: %q", got)
	}
	if got := policyURLFor("dev-1", "europe-central2-a", "pol-a"); got != "projects/dev-1/regions/europe-central2/resourcePolicies/pol-a" {
		t.Errorf("zonal policy URL: %q", got)
	}
}

// Discovery lists with no hints, and aggregated/disks mixes zonal and regional
// scopes. Each kind must emit only ids its own Read can resolve.
func TestAggregatedScopePrefixPerKind(t *testing.T) {
	if got := zonalAttachmentProvisioner().buildAttachmentNativeID("dev-1", "europe-central2-b", "d", "pol"); got !=
		"projects/dev-1/zones/europe-central2-b/disks/d/resourcePolicies/pol" {
		t.Errorf("zonal id: %q", got)
	}
	if got := regionalAttachmentProvisioner().buildAttachmentNativeID("dev-1", "europe-central2", "d", "pol"); got !=
		"projects/dev-1/regions/europe-central2/disks/d/resourcePolicies/pol" {
		t.Errorf("regional id: %q", got)
	}
}
