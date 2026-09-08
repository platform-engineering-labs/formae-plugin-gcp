// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package cloudrun

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// A target that sets region and leaves location unset is the shape
// /formae:connect writes. Cloud Run v2 has no zones and its locations *are*
// regions, so region is the location; interpolating the empty string produced
// "locations//" and a 400 on every discovery cycle.
func TestPathBuilderFallsBackToRegion(t *testing.T) {
	got := cloudRunPathBuilder(base.PathContext{
		Project: "p", Region: "us-central1", ResourceType: "services",
	})
	if want := "/projects/p/locations/us-central1/services"; got != want {
		t.Errorf("region fallback = %q, want %q", got, want)
	}

	// An explicit location still wins over region.
	got = cloudRunPathBuilder(base.PathContext{
		Project: "p", Region: "us-central1", Location: "europe-west1", ResourceType: "services",
	})
	if want := "/projects/p/locations/europe-west1/services"; got != want {
		t.Errorf("explicit location = %q, want %q", got, want)
	}
}

// Discovery lists with no properties, so a nested type has no parent to name.
// Cloud Run accepts "-" in the parent position, verified live 2026-09-08
// against project development-477117:
//
//	GET .../locations/europe-central2/services/-/revisions -> 200, and it
//	    returned a real revision under a real service, so the wildcard
//	    enumerates rather than merely being accepted.
//	GET .../locations/us-central1/jobs/-/executions        -> 200
//	GET .../locations/us-central1/jobs/-/executions/-/tasks -> 200
//
// Only one wildcard per path: locations/- together with services/- answers
// 400 "Request contains an invalid argument", so the location must stay
// concrete here.
func TestListWildcardsParentForNestedTypes(t *testing.T) {
	cases := map[string]string{
		"revisions":  "/projects/p/locations/us-central1/services/-/revisions",
		"executions": "/projects/p/locations/us-central1/jobs/-/executions",
		"tasks":      "/projects/p/locations/us-central1/jobs/-/executions/-/tasks",
	}
	for resourceType, want := range cases {
		got := cloudRunPathBuilder(base.PathContext{
			Project: "p", Location: "us-central1", ResourceType: resourceType, IsList: true,
		})
		if got != want {
			t.Errorf("%s list path = %q, want %q", resourceType, got, want)
		}
	}
}

// The wildcard is for discovery only. A caller that names the parent gets that
// parent, and a read/create/delete must never address "-".
func TestNamedParentBeatsWildcard(t *testing.T) {
	ctx := base.PathContext{
		Project: "p", Location: "us-central1", ResourceType: "revisions",
		ParentType: "services", ParentResource: "svc", IsList: true,
	}
	if got, want := cloudRunPathBuilder(ctx), "/projects/p/locations/us-central1/services/svc/revisions"; got != want {
		t.Errorf("named parent on list = %q, want %q", got, want)
	}

	ctx.IsList = false
	ctx.ResourceName = "rev1"
	if got, want := cloudRunPathBuilder(ctx), "/projects/p/locations/us-central1/services/svc/revisions/rev1"; got != want {
		t.Errorf("read path = %q, want %q", got, want)
	}
}

// A top-level type is unaffected: Cloud Run rejects locations/- for jobs
// (400) and answers 501 for workerPools, so no location wildcard is
// substituted for any type.
func TestNoLocationWildcardForTopLevelTypes(t *testing.T) {
	for _, resourceType := range []string{"services", "jobs", "workerPools"} {
		got := cloudRunPathBuilder(base.PathContext{
			Project: "p", Location: "us-central1", ResourceType: resourceType, IsList: true,
		})
		want := "/projects/p/locations/us-central1/" + resourceType
		if got != want {
			t.Errorf("%s list path = %q, want %q", resourceType, got, want)
		}
	}
}

// extractCloudRunNativeID used to substitute a hardcoded "us-central1" when the
// location was empty, which writes a native ID naming a region the resource is
// not in - silently, with no 404 to notice.
func TestNativeIDUsesRegionNotAHardcodedLocation(t *testing.T) {
	operation := map[string]interface{}{
		"name": "projects/p/locations/europe-west4/operations/op1",
	}
	got := extractCloudRunNativeID(operation, base.PathContext{
		Project: "p", Region: "europe-west4", ResourceType: "services", ResourceName: "svc",
	})
	if want := "projects/p/locations/europe-west4/services/svc"; got != want {
		t.Errorf("native ID = %q, want %q", got, want)
	}
}
