// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package main

import (
	"testing"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// StatusMessage surfaces as the CLI "reason:", so it must be cleared on
// in-progress and success results and kept only on failures.
func TestStripNonFailureMessage(t *testing.T) {
	cases := []struct {
		status  resource.OperationStatus
		wantMsg string
	}{
		{resource.OperationStatusInProgress, ""},
		{resource.OperationStatusPending, ""},
		{resource.OperationStatusSuccess, ""},
		{resource.OperationStatusFailure, "networks creation failed: quota exceeded"},
	}
	for _, c := range cases {
		pr := &resource.ProgressResult{
			OperationStatus: c.status,
			StatusMessage:   "networks creation failed: quota exceeded",
		}
		stripNonFailureMessage(pr)
		if pr.StatusMessage != c.wantMsg {
			t.Errorf("status %v: StatusMessage = %q, want %q", c.status, pr.StatusMessage, c.wantMsg)
		}
	}

	// A nil ProgressResult must not panic.
	stripNonFailureMessage(nil)
}
