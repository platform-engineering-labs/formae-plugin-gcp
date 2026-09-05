// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/platform-engineering-labs/formae/pkg/model"
)

// excludedFromDiscovery reports what discovery would do with this resource: run
// every filter that applies to the type and see whether any of them matches.
func excludedFromDiscovery(t *testing.T, resourceType, properties string) bool {
	t.Helper()
	filters := model.FiltersForType((&Plugin{}).DiscoveryFilters(), resourceType)
	for i := range filters {
		if filters[i].Excludes(json.RawMessage(properties)) {
			return true
		}
	}
	return false
}

func TestOwnershipMarkerExcludesALabelledResource(t *testing.T) {
	instance := `{"name":"formae-agent-1","labels":{"formae-owned":"true"}}`

	assert.True(t, excludedFromDiscovery(t, "GCP::Compute::Instance", instance))
}

func TestOwnershipMarkerLeavesEveryOtherResourceAlone(t *testing.T) {
	assert.False(t, excludedFromDiscovery(t, "GCP::Compute::Instance", `{"name":"theirs","labels":{"app":"formae-agent"}}`))
	assert.False(t, excludedFromDiscovery(t, "GCP::Compute::Instance", `{"name":"unlabelled"}`))
	assert.False(t, excludedFromDiscovery(t, "GCP::Compute::Instance", `{"name":"n","labels":{"formae-owned":"false"}}`))
}

// Project IAM bindings carry no labels, so connect's own grants are recognised
// by the member string, which names both formae's shared pool and its subject
// namespace.
func TestProjectIamBindingForFormaesPoolIsExcluded(t *testing.T) {
	ours := `{"project":"customer-project","role":"roles/viewer","member":"principalSet://iam.googleapis.com/projects/1234/locations/global/workloadIdentityPools/formae-ai/subject/fai:t/i"}`

	assert.True(t, excludedFromDiscovery(t, "GCP::IAM::ProjectIamMember", ours))
}

func TestProjectIamBindingForAnyoneElseStaysVisible(t *testing.T) {
	serviceAccount := `{"project":"customer-project","role":"roles/viewer","member":"serviceAccount:someone@customer-project.iam.gserviceaccount.com"}`
	otherPool := `{"project":"customer-project","role":"roles/viewer","member":"principalSet://iam.googleapis.com/projects/1234/locations/global/workloadIdentityPools/formae-ai-staging/subject/fai:t/i"}`
	otherNamespace := `{"project":"customer-project","role":"roles/viewer","member":"principalSet://iam.googleapis.com/projects/1234/locations/global/workloadIdentityPools/formae-ai/subject/someone-else"}`

	assert.False(t, excludedFromDiscovery(t, "GCP::IAM::ProjectIamMember", serviceAccount))
	assert.False(t, excludedFromDiscovery(t, "GCP::IAM::ProjectIamMember", otherPool))
	assert.False(t, excludedFromDiscovery(t, "GCP::IAM::ProjectIamMember", otherNamespace))
}

// The member condition has to search the whole document rather than $.member,
// because a filter selector does not evaluate against a scalar. Scoping the
// filter to this one type is what keeps that safe: its only other fields are
// project and role, neither of which can hold a pool path.
func TestPoolPathInAnotherResourcesFieldDoesNotLeakAcrossTypes(t *testing.T) {
	decoy := `{"name":"principalSet://iam.googleapis.com/projects/1234/locations/global/workloadIdentityPools/formae-ai/subject/fai:t/i"}`

	assert.False(t, excludedFromDiscovery(t, "GCP::Compute::Instance", decoy))
}
