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

// The exact envelope GCP returns for a disabled API, captured live from
// gkehub.googleapis.com against project development-477117 on 2026-09-10.
// Note that Errors is empty - the machine-readable reason lives in Details.
func serviceDisabledError() *googleapi.Error {
	return &googleapi.Error{
		Code:    403,
		Message: "GKE Hub API has not been used in project development-477117 before or it is disabled.",
		Details: []interface{}{
			map[string]interface{}{
				"@type":  "type.googleapis.com/google.rpc.ErrorInfo",
				"reason": "SERVICE_DISABLED",
				"domain": "googleapis.com",
				"metadata": map[string]interface{}{
					"service": "gkehub.googleapis.com",
				},
			},
		},
	}
}

func TestIsServiceDisabled(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"SERVICE_DISABLED in details", serviceDisabledError(), true},
		{
			"SERVICE_DISABLED in details, wrapped",
			fmt.Errorf("failed to list resources: %w", serviceDisabledError()),
			true,
		},
		{
			"legacy accessNotConfigured reason",
			&googleapi.Error{Code: 403, Errors: []googleapi.ErrorItem{{Reason: "accessNotConfigured"}}},
			true,
		},
		{
			"a real permission failure",
			&googleapi.Error{Code: 403, Errors: []googleapi.ErrorItem{{Reason: "forbidden"}}},
			false,
		},
		{
			"Cloud SQL's missing-parent 403",
			&googleapi.Error{Code: 403, Errors: []googleapi.ErrorItem{{Reason: "notAuthorized"}}},
			false,
		},
		{
			"an unrelated 403 carrying details",
			&googleapi.Error{Code: 403, Details: []interface{}{
				map[string]interface{}{"reason": "IAM_PERMISSION_DENIED"},
			}},
			false,
		},
		{
			"the same reason on a non-403",
			&googleapi.Error{Code: 400, Details: []interface{}{
				map[string]interface{}{"reason": "SERVICE_DISABLED"},
			}},
			false,
		},
		{"a message that merely mentions it", errors.New("SERVICE_DISABLED"), false},
		{"nil", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsServiceDisabled(tc.err); got != tc.want {
				t.Errorf("IsServiceDisabled() = %v, want %v", got, tc.want)
			}
		})
	}
}
