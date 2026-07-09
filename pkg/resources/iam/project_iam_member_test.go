// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package iam

import (
	"errors"
	"testing"

	"google.golang.org/api/googleapi"
)

// isMemberNotYetPropagated must match only the transient 400 "does not exist"
// that a not-yet-propagated principal produces — not other 400s, not other codes.
func TestIsMemberNotYetPropagated(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"propagation 400", &googleapi.Error{Code: 400, Message: "Service account foo@bar.iam.gserviceaccount.com does not exist."}, true},
		{"other 400", &googleapi.Error{Code: 400, Message: "Invalid role"}, false},
		{"403 does not exist", &googleapi.Error{Code: 403, Message: "does not exist"}, false},
		{"404", &googleapi.Error{Code: 404, Message: "not found"}, false},
		{"nil", nil, false},
		{"non-googleapi", errors.New("does not exist"), false},
		{"wrapped propagation 400", &wrapErr{&googleapi.Error{Code: 400, Message: "member does not exist"}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isMemberNotYetPropagated(c.err); got != c.want {
				t.Errorf("isMemberNotYetPropagated(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

type wrapErr struct{ e error }

func (w *wrapErr) Error() string { return "wrapped: " + w.e.Error() }
func (w *wrapErr) Unwrap() error { return w.e }
