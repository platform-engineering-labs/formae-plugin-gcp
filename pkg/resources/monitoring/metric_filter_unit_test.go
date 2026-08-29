// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package monitoring

import (
	"strings"
	"testing"
)

// Cloud Monitoring rejects OR within the metric prefix: the two-prefix filter
// answered HTTP 400, which failed the whole list and made sync tombstone a
// descriptor that was really there. Keep the filter a single predicate.
func TestUserOwnedMetricFilterHasNoOr(t *testing.T) {
	if strings.Contains(strings.ToUpper(userOwnedMetricFilter), " OR ") {
		t.Errorf("filter must be a single predicate, got %q", userOwnedMetricFilter)
	}
	if !strings.Contains(userOwnedMetricFilter, "custom.googleapis.com/") {
		t.Errorf("filter must keep user-owned descriptors, got %q", userOwnedMetricFilter)
	}
}
