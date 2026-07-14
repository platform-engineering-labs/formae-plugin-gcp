// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package sql

import (
	"errors"
	"testing"
)

// A database delete that fails because sessions haven't drained yet must be
// classified retryable so formae core re-runs it; other errors stay terminal.
func TestSQLRetryableError(t *testing.T) {
	re := SQLOperations.RetryableError
	if re == nil {
		t.Fatal("SQLOperations.RetryableError not set")
	}
	cases := map[string]bool{
		`operation failed: failed to delete database formae. Detail: pq: database "formae" is being accessed by other users.`: true,
		"is being accessed by other users":                    true,
		"operation failed: some other error":                  false,
	}
	for msg, want := range cases {
		if got := re(errors.New(msg)); got != want {
			t.Errorf("RetryableError(%q) = %v, want %v", msg, got, want)
		}
	}
	if re(nil) {
		t.Error("RetryableError(nil) should be false")
	}
}
