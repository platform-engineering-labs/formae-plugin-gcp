// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"

	"google.golang.org/api/googleapi"
)

// ErrorCode represents transport-level error classifications
type ErrorCode string

const (
	// ErrorCodeNone indicates no error occurred
	ErrorCodeNone ErrorCode = "NONE"

	// ErrorCodeInvalidInput indicates the request had invalid parameters (HTTP 400)
	ErrorCodeInvalidInput ErrorCode = "INVALID_INPUT"

	// ErrorCodeUnauthorized indicates authentication or authorization failure (HTTP 401, 403)
	ErrorCodeUnauthorized ErrorCode = "UNAUTHORIZED"

	// ErrorCodeResourceNotFound indicates the requested resource was not found (HTTP 404)
	ErrorCodeResourceNotFound ErrorCode = "RESOURCE_NOT_FOUND"

	// ErrorCodeAlreadyExists indicates the resource already exists or conflict (HTTP 409)
	ErrorCodeAlreadyExists ErrorCode = "ALREADY_EXISTS"

	// ErrorCodeConcurrencyConflict indicates optimistic locking failure (HTTP 412)
	ErrorCodeConcurrencyConflict ErrorCode = "CONCURRENCY_CONFLICT"

	// ErrorCodeThrottling indicates rate limiting (HTTP 429)
	ErrorCodeThrottling ErrorCode = "THROTTLING"

	// ErrorCodeInternalError indicates server-side errors (HTTP 500, 502, 503)
	ErrorCodeInternalError ErrorCode = "INTERNAL_ERROR"

	// ErrorCodeTimeout indicates operation exceeded time limit
	ErrorCodeTimeout ErrorCode = "TIMEOUT"

	// ErrorCodeNetworkFailure indicates a client-side transport failure with no
	// HTTP response (connection refused, DNS failure) — the endpoint is unreachable
	ErrorCodeNetworkFailure ErrorCode = "NETWORK_FAILURE"

	// ErrorCodeCancelled indicates operation was cancelled
	ErrorCodeCancelled ErrorCode = "CANCELLED"

	// ErrorCodeNotStabilized indicates resource is not yet ready
	ErrorCodeNotStabilized ErrorCode = "NOT_STABILIZED"

	// ErrorCodeUnknown indicates an unclassified error
	ErrorCodeUnknown ErrorCode = "UNKNOWN"
)

// Error represents a transport layer error with classification
type Error struct {
	Code       ErrorCode
	Message    string
	HTTPCode   int
	Underlying error
}

// Error implements the error interface
func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap allows errors.Is and errors.As to work
func (e *Error) Unwrap() error {
	return e.Underlying
}

// IsRetryable returns whether this error should be retried
func (e *Error) IsRetryable() bool {
	switch e.Code {
	case ErrorCodeThrottling,
		ErrorCodeInternalError,
		ErrorCodeNotStabilized,
		ErrorCodeConcurrencyConflict:
		return true
	default:
		return false
	}
}

// NewError creates a new transport error
func NewError(code ErrorCode, message string, underlying error) *Error {
	return &Error{
		Code:       code,
		Message:    message,
		Underlying: underlying,
	}
}

// ClassifyError maps a raw error to a transport ErrorCode
func ClassifyError(err error) ErrorCode {
	if err == nil {
		return ErrorCodeNone
	}

	// Check if already a transport error
	var te *Error
	if errors.As(err, &te) {
		return te.Code
	}

	// Auth/credential/token-fetch failures surface WITHOUT an HTTP response and,
	// when wrapped by net/http, become a *url.Error that satisfies net.Error.
	// Catch them BEFORE classifyTransportError below, or a healthy GCP endpoint
	// gets misread as unreachable over a stale credential.
	if isAuthError(err) {
		return ErrorCodeUnauthorized
	}

	// A googleapi.Error means the endpoint answered with an HTTP status.
	// errors.As unwraps recursively, including fmt.wrapError.
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		switch gerr.Code {
		case 400:
			// Check for specific 400 error types
			return classifyBadRequestError(gerr)
		case 401, 403:
			return ErrorCodeUnauthorized
		case 404:
			return ErrorCodeResourceNotFound
		case 409:
			return ErrorCodeAlreadyExists
		case 412:
			return ErrorCodeConcurrencyConflict
		case 429:
			return ErrorCodeThrottling
		case 500, 502, 503:
			return ErrorCodeInternalError
		default:
			return ErrorCodeUnknown
		}
	}

	// No HTTP response: a client-side transport failure (endpoint unreachable).
	// This is the health signal formae's target reaper keys off.
	if code, ok := classifyTransportError(err); ok {
		return code
	}

	// String-match fallback for wrapped errors that carry no typed value.
	errMsg := err.Error()
	if containsAny(errMsg, "404", "not found", "does not exist") {
		return ErrorCodeResourceNotFound
	}
	if containsAny(errMsg, "401", "403", "unauthorized", "permission denied") {
		return ErrorCodeUnauthorized
	}
	if containsAny(errMsg, "409", "already exists", "conflict") {
		return ErrorCodeAlreadyExists
	}
	if containsAny(errMsg, "429", "rate limit", "quota exceeded") {
		return ErrorCodeThrottling
	}
	if containsAny(errMsg, "500", "502", "503", "internal error") {
		return ErrorCodeInternalError
	}

	// Not recognizable - unknown
	return ErrorCodeUnknown
}

// isAuthError reports whether err is an OAuth/credential/token-retrieval failure
// rather than a problem reaching the GCP endpoint. These fail before (or
// independent of) any HTTP response, so they must never be classified as a
// network/timeout transport failure — the endpoint is typically healthy and the
// fix is to refresh credentials, not to reap the target. isReauthError
// (transport.go) covers the Workspace `invalid_rapt` reauth case.
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	if isReauthError(err) {
		return true
	}
	return containsAny(err.Error(),
		"oauth2: cannot fetch token",
		"could not find default credentials",
		"google: could not fetch",
		"failed to obtain token",
		"invalid_grant",
	)
}

// classifyTransportError maps a client-side transport failure (no HTTP response
// received: connection refused, DNS failure, dial/read timeout) to the health
// signal the formae target reaper keys off. Returns ok=false for anything else.
//
// It matches CONCRETE net types (unwrapping the *url.Error the http client wraps
// around the RoundTripper error) and the deadline sentinels — never the
// net.Error interface, which an auth-wrapped *url.Error would also satisfy. Auth
// failures are excluded upstream in ClassifyError before this runs.
func classifyTransportError(err error) (ErrorCode, bool) {
	if err == nil {
		return "", false
	}

	// A genuine dial/read deadline against the endpoint.
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return ErrorCodeTimeout, true
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Timeout() {
			return ErrorCodeTimeout, true
		}
		return ErrorCodeNetworkFailure, true
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return ErrorCodeNetworkFailure, true
	}

	if errors.Is(err, syscall.ECONNREFUSED) {
		return ErrorCodeNetworkFailure, true
	}

	return "", false
}

// classifyBadRequestError provides finer-grained classification of 400 errors
func classifyBadRequestError(gerr *googleapi.Error) ErrorCode {
	// Check for resource not ready errors
	if containsAny(gerr.Body, "resourceNotReady", "not ready", "not in ready state") {
		return ErrorCodeNotStabilized
	}

	// Default to invalid input
	return ErrorCodeInvalidInput
}

// WrapError wraps an error with transport error classification
func WrapError(err error, contextMessage string) *Error {
	if err == nil {
		return nil
	}

	// If already a transport error, just update message
	var te *Error
	if errors.As(err, &te) {
		return &Error{
			Code:       te.Code,
			Message:    contextMessage,
			HTTPCode:   te.HTTPCode,
			Underlying: te.Underlying,
		}
	}

	code := ClassifyError(err)
	httpCode := 0
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		httpCode = gerr.Code
	}

	// Extract clean message from underlying error and combine with context
	cleanMsg := extractCleanMessage(err)
	finalMessage := contextMessage
	if cleanMsg != "" {
		finalMessage = fmt.Sprintf("%s: %s", contextMessage, cleanMsg)
	}

	return &Error{
		Code:       code,
		Message:    finalMessage,
		HTTPCode:   httpCode,
		Underlying: err,
	}
}

// IsRetryableError determines if an error should be retried
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check if transport error using errors.As
	var te *Error
	if errors.As(err, &te) {
		return te.IsRetryable()
	}

	// Classify and check
	code := ClassifyError(err)
	return (&Error{Code: code}).IsRetryable()
}

// extractCleanMessage extracts the core error message from googleapi errors
func extractCleanMessage(err error) string {
	if err == nil {
		return ""
	}

	// Try to extract googleapi.Error by unwrapping all layers
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		return buildGoogleAPIErrorMessage(gerr)
	}

	// Recursively unwrap to find any useful message
	current := err
	for current != nil {
		// Try googleapi.Error again at each level
		if errors.As(current, &gerr) {
			return buildGoogleAPIErrorMessage(gerr)
		}

		// Try to unwrap
		unwrapper, ok := current.(interface{ Unwrap() error })
		if !ok {
			break
		}
		current = unwrapper.Unwrap()
	}

	return ""
}

// buildGoogleAPIErrorMessage constructs a detailed error message from googleapi.Error
func buildGoogleAPIErrorMessage(gerr *googleapi.Error) string {
	if gerr == nil {
		return ""
	}

	// Start with the main message
	msg := gerr.Message
	if msg == "" {
		msg = fmt.Sprintf("HTTP %d error", gerr.Code)
	}

	// Check for field-specific errors in the Details
	// GCP APIs often return detailed field violations in BadRequest details
	if len(gerr.Details) > 0 {
		for _, detail := range gerr.Details {
			if detailMap, ok := detail.(map[string]interface{}); ok {
				// Extract field violations (common in Container API errors)
				if fieldViolations, ok := detailMap["fieldViolations"].([]interface{}); ok {
					for _, fv := range fieldViolations {
						if fvMap, ok := fv.(map[string]interface{}); ok {
							field := fvMap["field"]
							desc := fvMap["description"]
							if field != nil && field != "" {
								msg = fmt.Sprintf("%s [field '%v': %v]", msg, field, desc)
							} else if desc != nil {
								msg = fmt.Sprintf("%s [%v]", msg, desc)
							}
						}
					}
				}
				// Also check for preconditionFailure violations
				if violations, ok := detailMap["violations"].([]interface{}); ok {
					for _, v := range violations {
						if vMap, ok := v.(map[string]interface{}); ok {
							vType := vMap["type"]
							subject := vMap["subject"]
							desc := vMap["description"]
							if desc != nil {
								if subject != nil {
									msg = fmt.Sprintf("%s [%v: %v - %v]", msg, vType, subject, desc)
								} else {
									msg = fmt.Sprintf("%s [%v]", msg, desc)
								}
							}
						}
					}
				}
			}
		}
	}

	// Also check Errors array for additional context
	if len(gerr.Errors) > 0 {
		for _, e := range gerr.Errors {
			if e.Message != "" && e.Message != gerr.Message {
				msg = fmt.Sprintf("%s - %s", msg, e.Message)
			}
			if e.Reason != "" {
				msg = fmt.Sprintf("%s [reason: %s]", msg, e.Reason)
			}
		}
	}

	// If Body contains more details, try to extract them
	if gerr.Body != "" && msg == "Request contains an invalid argument." {
		// Try to parse body for more details
		var bodyData map[string]interface{}
		if err := json.Unmarshal([]byte(gerr.Body), &bodyData); err == nil {
			if errorObj, ok := bodyData["error"].(map[string]interface{}); ok {
				if status, ok := errorObj["status"].(string); ok && status != "" {
					msg = fmt.Sprintf("%s [status: %s]", msg, status)
				}
			}
		}
	}

	return msg
}

// Helper function to check if string contains any of the substrings (case-insensitive)
func containsAny(s string, substrings ...string) bool {
	// Convert to lowercase for case-insensitive comparison
	sLower := ""
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		sLower += string(c)
	}

	for _, substr := range substrings {
		// Convert substring to lowercase
		substrLower := ""
		for i := 0; i < len(substr); i++ {
			c := substr[i]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			substrLower += string(c)
		}

		// Check if sLower contains substrLower
		if len(sLower) >= len(substrLower) {
			for i := 0; i <= len(sLower)-len(substrLower); i++ {
				if sLower[i:i+len(substrLower)] == substrLower {
					return true
				}
			}
		}
	}
	return false
}
