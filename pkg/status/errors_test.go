// © 2026 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package status

import (
	"fmt"
	"testing"

	"github.com/googleapis/gax-go/v2/apierror"
	"google.golang.org/api/googleapi"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func userProjectDenied() *googleapi.Error {
	return &googleapi.Error{
		Code:    403,
		Message: "Caller does not have required permission to use project p.",
		Details: []interface{}{
			map[string]interface{}{
				"@type":  "type.googleapis.com/google.rpc.ErrorInfo",
				"reason": "USER_PROJECT_DENIED",
				"domain": "googleapis.com",
			},
		},
	}
}

// TestMapGCPError_UserProjectDenied covers the SDK-client path: a quota-project
// refusal must be retried like the transport path does, while any other 403
// stays terminal.
func TestMapGCPError_UserProjectDenied(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want resource.OperationErrorCode
	}{
		{"googleapi", userProjectDenied(), resource.OperationErrorCodeThrottling},
		{"wrapped", fmt.Errorf("creating table: %w", userProjectDenied()), resource.OperationErrorCodeThrottling},
		{"plain 403", &googleapi.Error{Code: 403, Message: "denied"}, resource.OperationErrorCodeAccessDenied},
	}

	if apiErr, ok := apierror.FromError(userProjectDenied()); ok {
		cases = append(cases, struct {
			name string
			err  error
			want resource.OperationErrorCode
		}{"apierror", apiErr, resource.OperationErrorCodeThrottling})
	} else {
		t.Fatal("apierror.FromError did not wrap the googleapi error")
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MapGCPError(tc.err); got != tc.want {
				t.Errorf("MapGCPError() = %s, want %s", got, tc.want)
			}
		})
	}
}
