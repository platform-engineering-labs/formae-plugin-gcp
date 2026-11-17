// © 2026 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package transport

import (
	"fmt"
	"testing"

	"google.golang.org/api/googleapi"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// userProjectDeniedError is the refusal Service Usage returned to a federated
// principal five seconds after `formae connect gcp` granted it roles/editor,
// in the oidc-credential e2e on 2026-11-17.
func userProjectDeniedError() *googleapi.Error {
	return &googleapi.Error{
		Code: 403,
		Message: "Caller does not have required permission to use project formae-e2e-oidc. " +
			"Grant the caller the roles/serviceusage.serviceUsageConsumer role, or a custom role with the " +
			"serviceusage.services.use permission, by visiting " +
			"https://console.developers.google.com/iam-admin/iam?project=formae-e2e-oidc and then retry. " +
			"Propagation of the new permission may take a few minutes.",
		Details: []interface{}{
			map[string]interface{}{
				"@type":  "type.googleapis.com/google.rpc.ErrorInfo",
				"reason": "USER_PROJECT_DENIED",
				"domain": "googleapis.com",
				"metadata": map[string]interface{}{
					"consumer": "projects/formae-e2e-oidc",
					"service":  "iam.googleapis.com",
				},
			},
		},
	}
}

func TestIsUserProjectDenied(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"user project denied", userProjectDeniedError(), true},
		{"wrapped", fmt.Errorf("failed to create service account: %w", userProjectDeniedError()), true},
		{"plain permission denied", &googleapi.Error{Code: 403, Message: "Permission 'iam.serviceAccounts.create' denied"}, false},
		{"service disabled", serviceDisabledError(), false},
		{
			"reason on a non-403",
			&googleapi.Error{Code: 400, Details: []interface{}{map[string]interface{}{"reason": "USER_PROJECT_DENIED"}}},
			false,
		},
		{"message only", &googleapi.Error{Code: 403, Message: "Caller does not have required permission to use project x"}, false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsUserProjectDenied(tc.err); got != tc.want {
				t.Errorf("IsUserProjectDenied() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestClassifyError_UserProjectDeniedIsRetried guards the outcome that matters:
// the refusal reaches formae as a recoverable code with exponential backoff, not
// as the terminal AccessDenied every other 403 maps to.
func TestClassifyError_UserProjectDeniedIsRetried(t *testing.T) {
	err := fmt.Errorf("failed to create service account: %w", userProjectDeniedError())

	code := ClassifyError(err)
	if code != ErrorCodePermissionPropagating {
		t.Fatalf("ClassifyError() = %s, want %s", code, ErrorCodePermissionPropagating)
	}
	if !IsRetryableError(err) {
		t.Errorf("IsRetryableError() = false, want true")
	}
	wrapped := WrapError(err, "failed to create service account")
	if got := ToResourceErrorCode(wrapped.Code); got != resource.OperationErrorCodeThrottling {
		t.Errorf("ToResourceErrorCode() = %s, want %s", got, resource.OperationErrorCodeThrottling)
	}
	if !resource.IsRecoverable(ToResourceErrorCode(wrapped.Code)) {
		t.Errorf("mapped code %s is not recoverable in formae", ToResourceErrorCode(wrapped.Code))
	}

	plain := &googleapi.Error{Code: 403, Message: "Permission 'iam.serviceAccounts.create' denied"}
	if got := ClassifyError(plain); got != ErrorCodeUnauthorized {
		t.Errorf("a plain 403 must stay Unauthorized, got %s", got)
	}
}
