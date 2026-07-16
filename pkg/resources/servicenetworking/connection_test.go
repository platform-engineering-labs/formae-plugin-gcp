// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package servicenetworking

import "testing"

// The PSA connection delete must treat the transient "producer still using"
// error as retryable (NotStabilized), but any other failure as terminal.
func TestIsProducerStillUsing(t *testing.T) {
	cases := map[string]bool{
		"Failed to delete connection; Producer services (e.g. CloudSQL, Cloud Memstore, etc.) are still using this connection.": true,
		"still using this connection": true,
		"some other error":            false,
		"":                            false,
	}
	for msg, want := range cases {
		if got := isProducerStillUsing(msg); got != want {
			t.Errorf("isProducerStillUsing(%q) = %v, want %v", msg, got, want)
		}
	}
}
