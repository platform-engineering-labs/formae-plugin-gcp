// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"

	"google.golang.org/api/googleapi"
)

// timeoutErr reports itself as a timeout, used to build a net.OpError whose
// Timeout() is true (the shape a dial/read deadline takes at the socket layer).
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

// urlErr wraps inner the way net/http's client wraps any RoundTripper error —
// this is what actually reaches the plugin from a GCP API request.
func urlErr(inner error) *url.Error {
	return &url.Error{Op: "Get", URL: "https://compute.googleapis.com/x", Err: inner}
}

func TestClassifyError_TransportAndAuth(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ErrorCode
	}{
		{"nil", nil, ErrorCodeNone},
		// --- Half B: client-side transport failures → unreachable signal.
		{
			"connection refused wrapped in url.Error",
			urlErr(&net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}),
			ErrorCodeNetworkFailure,
		},
		{
			"bare connection-refused OpError",
			&net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED},
			ErrorCodeNetworkFailure,
		},
		{
			"DNS failure",
			urlErr(&net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "no such host", Name: "x", IsNotFound: true}}),
			ErrorCodeNetworkFailure,
		},
		{
			"dial/read timeout",
			urlErr(&net.OpError{Op: "dial", Net: "tcp", Err: timeoutErr{}}),
			ErrorCodeTimeout,
		},
		{
			"context deadline",
			urlErr(context.DeadlineExceeded),
			ErrorCodeTimeout,
		},
		{
			"os deadline",
			urlErr(os.ErrDeadlineExceeded),
			ErrorCodeTimeout,
		},
		// --- Half A / the auth trap: token failures wrap in url.Error (satisfy
		// net.Error) but the endpoint is healthy — must be Unauthorized, never
		// a network/timeout signal.
		{
			"reauth invalid_rapt is Unauthorized not network",
			urlErr(fmt.Errorf("reauth related error (invalid_rapt)")),
			ErrorCodeUnauthorized,
		},
		{
			"oauth2 cannot fetch token is Unauthorized",
			urlErr(fmt.Errorf("oauth2: cannot fetch token: 400 Bad Request")),
			ErrorCodeUnauthorized,
		},
		{
			"missing default credentials is Unauthorized",
			fmt.Errorf("google: could not find default credentials"),
			ErrorCodeUnauthorized,
		},
		{
			"invalid_grant (stale refresh token) is Unauthorized",
			urlErr(fmt.Errorf("oauth2: %q", "invalid_grant")),
			ErrorCodeUnauthorized,
		},
		{
			"auth-path deadline is Unauthorized not timeout (hung IdP, healthy endpoint)",
			urlErr(fmt.Errorf("failed to obtain token: %w", context.DeadlineExceeded)),
			ErrorCodeUnauthorized,
		},
		// --- googleapi HTTP responses keep their existing mapping.
		{
			"http 403 is Unauthorized",
			&googleapi.Error{Code: 403, Message: "permission denied"},
			ErrorCodeUnauthorized,
		},
		{
			"http 404 is not found",
			&googleapi.Error{Code: 404, Message: "not found"},
			ErrorCodeResourceNotFound,
		},
		// --- Application error stays unknown.
		{
			"unrecognized application error is unknown",
			errors.New("the server rejected our request for an unknown reason"),
			ErrorCodeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyError(tt.err); got != tt.want {
				t.Errorf("ClassifyError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractCleanMessage(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
		{
			name: "direct googleapi error",
			err: &googleapi.Error{
				Code:    400,
				Message: "This operation does not support custom billing projects at this time.",
			},
			expected: "This operation does not support custom billing projects at this time.",
		},
		{
			name: "wrapped googleapi error",
			err: fmt.Errorf("non-retryable error: %w", &googleapi.Error{
				Code:    400,
				Message: "This operation does not support custom billing projects at this time.",
			}),
			expected: "This operation does not support custom billing projects at this time.",
		},
		{
			name: "double wrapped googleapi error",
			err: fmt.Errorf("outer wrap: %w", fmt.Errorf("inner wrap: %w", &googleapi.Error{
				Code:    400,
				Message: "This operation does not support custom billing projects at this time.",
			})),
			expected: "This operation does not support custom billing projects at this time.",
		},
		{
			name:     "non-googleapi error",
			err:      fmt.Errorf("some other error"),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCleanMessage(tt.err)
			if result != tt.expected {
				t.Errorf("extractCleanMessage() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestWrapError(t *testing.T) {
	googleErr := &googleapi.Error{
		Code:    400,
		Message: "This operation does not support custom billing projects at this time.",
	}

	// Simulate what happens in retry.go
	wrappedErr := fmt.Errorf("non-retryable error: %w", googleErr)

	// Now wrap it with our transport error
	transportErr := WrapError(wrappedErr, "failed to create resource")

	// Check that the message includes both context and the clean googleapi message
	expectedMessage := "failed to create resource: This operation does not support custom billing projects at this time."
	if transportErr.Message != expectedMessage {
		t.Errorf("WrapError() Message = %q, want %q", transportErr.Message, expectedMessage)
	}

	// Check the Error() method output
	expectedError := "INVALID_INPUT: failed to create resource: This operation does not support custom billing projects at this time."
	if transportErr.Error() != expectedError {
		t.Errorf("WrapError().Error() = %q, want %q", transportErr.Error(), expectedError)
	}
}
