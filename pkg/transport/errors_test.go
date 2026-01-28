// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package transport

import (
	"fmt"
	"testing"

	"google.golang.org/api/googleapi"
)

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
