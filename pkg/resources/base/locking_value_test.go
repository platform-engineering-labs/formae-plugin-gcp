// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package base

import (
	"encoding/json"
	"testing"
)

// Most GCP etags are strings, but some are numbers — Dataproc's
// workflowTemplates.version is an int. JSON decodes every number to float64, so
// a naive render would produce "2" for a string etag and "2" (not "2.0") here.
func TestLockingValueString(t *testing.T) {
	cases := map[string]struct {
		in   interface{}
		want string
	}{
		"string etag":  {"BwXhc9E=", "BwXhc9E="},
		"json number":  {float64(2), "2"},
		"large number": {float64(1234567890), "1234567890"},
		"int":          {int(7), "7"},
		"int64":        {int64(8), "8"},
		"json.Number":  {json.Number("3"), "3"},
		"absent":       {nil, ""},
		"unexpected":   {[]interface{}{1}, ""},
	}
	for name, c := range cases {
		if got := lockingValueString(c.in); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}
