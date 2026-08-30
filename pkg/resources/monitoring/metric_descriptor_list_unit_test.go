// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package monitoring

import (
	"strings"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// metricDescriptors.list returns every descriptor the project can see - well
// over a thousand built-in ones - so a custom metric may not be on the first
// page at all. Only the two user-owned prefixes can be created or deleted, so
// only those belong in discovery.
func TestMetricDescriptorListIsFilteredToUserOwnedMetrics(t *testing.T) {
	got := monitoringPathBuilder(base.PathContext{
		Project: "p", ResourceType: "metricDescriptors", IsList: true,
	})
	if !strings.HasPrefix(got, "/projects/p/metricDescriptors?filter=") {
		t.Fatalf("list path = %q", got)
	}
	// One prefix, not two. Cloud Monitoring rejects
	// "metric.type = starts_with(...) OR metric.type = starts_with(...)" with
	// HTTP 400, "Within the 'metric' prefix, OR can only be used to connect a
	// list of 'labels'", and a rejected filter fails the whole list - which read
	// downstream as "the resource is gone" and had sync tombstone a descriptor
	// that was really there.
	if !strings.Contains(got, "custom.googleapis.com") && !strings.Contains(got, "custom%2Egoogleapis%2Ecom") {
		t.Errorf("filter should keep user-owned descriptors, got %q", got)
	}
	if strings.Contains(strings.ToUpper(got), "+OR+") || strings.Contains(strings.ToUpper(got), "%20OR%20") {
		t.Errorf("filter must stay a single predicate, got %q", got)
	}
}

// The filter is a list concern: create posts to the collection and read
// addresses one descriptor, and neither may carry it.
func TestMetricDescriptorNonListPathsCarryNoFilter(t *testing.T) {
	create := monitoringPathBuilder(base.PathContext{
		Project: "p", ResourceType: "metricDescriptors",
	})
	if want := "/projects/p/metricDescriptors"; create != want {
		t.Errorf("create path = %q, want %q", create, want)
	}

	read := monitoringPathBuilder(base.PathContext{
		Project: "p", ResourceType: "metricDescriptors", ResourceName: "custom.googleapis.com/x",
	})
	if strings.Contains(read, "filter=") {
		t.Errorf("read path must carry no filter, got %q", read)
	}
}

// Other monitoring collections must not grow a filter.
func TestOtherMonitoringListsAreUnfiltered(t *testing.T) {
	got := monitoringPathBuilder(base.PathContext{
		Project: "p", ResourceType: "alertPolicies", IsList: true,
	})
	if want := "/projects/p/alertPolicies"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
