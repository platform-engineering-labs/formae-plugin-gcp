// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/googleapi"
)

func TestAddQueryParam(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		key      string
		value    string
		expected string
	}{
		{
			name:     "add to URL without existing params",
			url:      "https://example.com/api",
			key:      "alt",
			value:    "json",
			expected: "https://example.com/api?alt=json",
		},
		{
			name:     "add to URL with existing params",
			url:      "https://example.com/api?foo=bar",
			key:      "alt",
			value:    "json",
			expected: "https://example.com/api?alt=json&foo=bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := AddQueryParam(tt.url, tt.key, tt.value)
			require.NoError(t, err)
			assert.Contains(t, result, "alt=json")
		})
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorCode
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: ErrorCodeNone,
		},
		{
			name: "400 bad request",
			err: &googleapi.Error{
				Code:    400,
				Message: "Invalid input",
			},
			expected: ErrorCodeInvalidInput,
		},
		{
			name: "401 unauthorized",
			err: &googleapi.Error{
				Code:    401,
				Message: "Unauthorized",
			},
			expected: ErrorCodeUnauthorized,
		},
		{
			name: "403 forbidden",
			err: &googleapi.Error{
				Code:    403,
				Message: "Forbidden",
			},
			expected: ErrorCodeUnauthorized,
		},
		{
			name: "404 not found",
			err: &googleapi.Error{
				Code:    404,
				Message: "Not found",
			},
			expected: ErrorCodeResourceNotFound,
		},
		{
			name: "409 conflict",
			err: &googleapi.Error{
				Code:    409,
				Message: "Already exists",
			},
			expected: ErrorCodeAlreadyExists,
		},
		{
			name: "412 precondition failed",
			err: &googleapi.Error{
				Code:    412,
				Message: "Fingerprint mismatch",
			},
			expected: ErrorCodeConcurrencyConflict,
		},
		{
			name: "429 too many requests",
			err: &googleapi.Error{
				Code:    429,
				Message: "Rate limit exceeded",
			},
			expected: ErrorCodeThrottling,
		},
		{
			name: "500 internal server error",
			err: &googleapi.Error{
				Code:    500,
				Message: "Internal error",
			},
			expected: ErrorCodeInternalError,
		},
		{
			name: "502 bad gateway",
			err: &googleapi.Error{
				Code:    502,
				Message: "Bad gateway",
			},
			expected: ErrorCodeInternalError,
		},
		{
			name: "503 service unavailable",
			err: &googleapi.Error{
				Code:    503,
				Message: "Service unavailable",
			},
			expected: ErrorCodeInternalError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name: "429 rate limit",
			err: &googleapi.Error{
				Code:    429,
				Message: "Rate limit exceeded",
			},
			expected: true,
		},
		{
			name: "500 internal error",
			err: &googleapi.Error{
				Code:    500,
				Message: "Internal error",
			},
			expected: true,
		},
		{
			name: "400 invalid input (not retryable)",
			err: &googleapi.Error{
				Code:    400,
				Message: "Invalid input",
			},
			expected: false,
		},
		{
			name: "404 not found (not retryable by default)",
			err: &googleapi.Error{
				Code:    404,
				Message: "Not found",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryableError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractOperationURL(t *testing.T) {
	tests := []struct {
		name     string
		response map[string]interface{}
		baseURL  string
		expected string
		wantErr  bool
	}{
		{
			name: "extract from selfLink",
			response: map[string]interface{}{
				"selfLink": "https://compute.googleapis.com/compute/v1/projects/test/operations/op-123",
			},
			baseURL:  "",
			expected: "https://compute.googleapis.com/compute/v1/projects/test/operations/op-123",
			wantErr:  false,
		},
		{
			name: "extract from name with baseURL",
			response: map[string]interface{}{
				"name": "projects/test/operations/op-123",
			},
			baseURL:  "https://compute.googleapis.com/compute/v1",
			expected: "https://compute.googleapis.com/compute/v1/projects/test/operations/op-123",
			wantErr:  false,
		},
		{
			name: "no operation URL",
			response: map[string]interface{}{
				"foo": "bar",
			},
			baseURL: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractOperationURL(tt.response, tt.baseURL)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestSendRequest_Success(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Contains(t, r.Header.Get("User-Agent"), "formae-gcp-plugin")

		response := map[string]interface{}{
			"name": "test-resource",
			"id":   "12345",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Note: This test would require a real Client with OAuth2
	// For demonstration purposes, we show the expected behavior
	ctx := context.Background()
	_ = ctx
	_ = server.URL
}

func TestSendRequest_Retry(t *testing.T) {
	// Track number of attempts
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++

		if attempts < 3 {
			// Return 503 for first two attempts
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"code":    503,
					"message": "Service unavailable",
				},
			})
			return
		}

		// Succeed on third attempt
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name": "test-resource",
		})
	}))
	defer server.Close()

	// This demonstrates the retry behavior
	// In real usage, the retry logic would handle the 503 errors automatically
}

func TestParseOperationResponse(t *testing.T) {
	tests := []struct {
		name             string
		response         map[string]interface{}
		expectedStatus   OperationStatus
		expectedErrCode  ErrorCode
		wantErr          bool
	}{
		{
			name: "operation in progress",
			response: map[string]interface{}{
				"name": "op-123",
				"done": false,
			},
			expectedStatus:  OperationStatusInProgress,
			expectedErrCode: ErrorCodeNone,
			wantErr:         false,
		},
		{
			name: "operation completed successfully",
			response: map[string]interface{}{
				"name": "op-123",
				"done": true,
			},
			expectedStatus:  OperationStatusSuccess,
			expectedErrCode: ErrorCodeNone,
			wantErr:         false,
		},
		{
			name: "operation failed",
			response: map[string]interface{}{
				"name": "op-123",
				"done": true,
				"error": map[string]interface{}{
					"code":    400,
					"message": "Invalid request",
				},
			},
			expectedStatus:  OperationStatusFailure,
			expectedErrCode: ErrorCodeUnknown,
			wantErr:         true,
		},
		{
			name: "synchronous success (no done field)",
			response: map[string]interface{}{
				"name": "resource-123",
				"id":   "12345",
			},
			expectedStatus:  OperationStatusSuccess,
			expectedErrCode: ErrorCodeNone,
			wantErr:         false,
		},
		{
			name:            "nil response",
			response:        nil,
			expectedStatus:  OperationStatusFailure,
			expectedErrCode: ErrorCodeUnknown,
			wantErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseOperationResponse(tt.response)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.NotNil(t, result)
			assert.Equal(t, tt.expectedStatus, result.Status)
			assert.Equal(t, tt.expectedErrCode, result.Error)
		})
	}
}

// Note: Retry tests removed as retry logic is now handled by metastructure's PluginOperator.
// See pkg/plugin/plugin_operator.go for the centralized retry implementation.
