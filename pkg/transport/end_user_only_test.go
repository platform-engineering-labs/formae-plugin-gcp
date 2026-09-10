// © 2026 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package transport

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/api/googleapi"
)

func endUserOnlyError() *googleapi.Error {
	return &googleapi.Error{
		Code:    403,
		Message: "Authentication error. Invalid end user or user type not supported.",
	}
}

func TestIsEndUserOnly(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"the answer itself", endUserOnlyError(), true},
		{
			"wrapped, as List wraps it",
			fmt.Errorf("failed to list resources: %w", endUserOnlyError()),
			true,
		},
		{
			"a real permission failure",
			&googleapi.Error{Code: 403, Message: "Permission 'logging.queries.list' denied on resource ..."},
			false,
		},
		{
			"Cloud SQL's missing-parent 403",
			&googleapi.Error{Code: 403, Message: "The client is not authorized to make this request."},
			false,
		},
		{
			"a disabled API - handled by IsServiceDisabled, not here",
			serviceDisabledError(),
			false,
		},
		{
			"the same sentence on a non-403",
			&googleapi.Error{Code: 401, Message: "Authentication error. Invalid end user or user type not supported."},
			false,
		},
		{"the sentence with no HTTP error at all", errors.New("Invalid end user or user type not supported"), false},
		{"nil", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsEndUserOnly(tc.err); got != tc.want {
				t.Errorf("IsEndUserOnly() = %v, want %v", got, tc.want)
			}
		})
	}
}
