// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package memcache

import "testing"

// A forma may name a network three ways and memcache accepts only one:
// "projects/{project}/global/networks/{network}". A reference to another
// resource's network property resolves to a full self link, which the API
// rejects with "Invalid format for authorized network name."
func TestNormalizeNetwork(t *testing.T) {
	want := "projects/p/global/networks/n"
	for _, in := range []string{
		"n",
		"projects/p/global/networks/n",
		"https://www.googleapis.com/compute/v1/projects/p/global/networks/n",
	} {
		if got := normalizeNetwork(in, "p"); got != want {
			t.Errorf("normalizeNetwork(%q) = %q, want %q", in, got, want)
		}
	}
}
