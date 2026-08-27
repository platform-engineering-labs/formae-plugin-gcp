// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package monitoring

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// dashboards.create answers with the project number and dashboards.list with
// the project id, for the same dashboard. Both have to reduce to one native ID
// or discovery reports a managed resource a second time as an unmanaged one.
func TestNativeIDUsesConfiguredProject(t *testing.T) {
	ctx := base.PathContext{Project: "my-project", ResourceType: "dashboards"}
	want := "projects/my-project/dashboards/dash-1"

	fromCreate := extractMonitoringNativeID(
		map[string]interface{}{"name": "projects/989754770009/dashboards/dash-1"}, ctx)
	if fromCreate != want {
		t.Errorf("create form = %q, want %q", fromCreate, want)
	}

	fromList := extractMonitoringNativeID(
		map[string]interface{}{"name": "projects/my-project/dashboards/dash-1"}, ctx)
	if fromList != want {
		t.Errorf("list form = %q, want %q", fromList, want)
	}
}

// With no configured project there is nothing to normalise to; the path from
// the API is still better than none.
func TestNativeIDKeepsPathWithoutProject(t *testing.T) {
	got := extractMonitoringNativeID(
		map[string]interface{}{"name": "projects/989754770009/dashboards/dash-1"},
		base.PathContext{})
	if got != "projects/989754770009/dashboards/dash-1" {
		t.Errorf("got %q, want the path unchanged", got)
	}
}
