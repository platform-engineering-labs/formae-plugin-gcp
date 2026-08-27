// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package base

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
)

// A poll that could not reach the API must not fail the operation: the caller
// re-issues the create, which then collides with what the first attempt built.
func TestTransientPollErrorsKeepPolling(t *testing.T) {
	for _, code := range []transport.ErrorCode{
		transport.ErrorCodeNetworkFailure,
		transport.ErrorCodeTimeout,
		transport.ErrorCodeThrottling,
		transport.ErrorCodeInternalError,
		// An error the transport could not classify says nothing about the
		// operation either - a burst of these once failed eleven jobs whose
		// operations were fine.
		transport.ErrorCodeUnknown,
		transport.ErrorCodeCancelled,
	} {
		if !isTransientPollError(code) {
			t.Errorf("%s should keep polling", code)
		}
	}
}

// A definitive answer still fails, or the operation would poll until timeout.
func TestDefinitivePollErrorsFail(t *testing.T) {
	for _, code := range []transport.ErrorCode{
		transport.ErrorCodeResourceNotFound,
		transport.ErrorCodeUnauthorized,
		transport.ErrorCodeInvalidInput,
		transport.ErrorCodeAlreadyExists,
	} {
		if isTransientPollError(code) {
			t.Errorf("%s should fail, not keep polling", code)
		}
	}
}
