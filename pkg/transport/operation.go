// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package transport

import (
	"context"
	"fmt"
	"time"
)

const (
	DefaultOperationTimeout      = 10 * time.Minute
	DefaultOperationPollInterval = 5 * time.Second
)

// OperationStatus represents the status of an operation
type OperationStatus string

const (
	OperationStatusPending    OperationStatus = "PENDING"
	OperationStatusInProgress OperationStatus = "IN_PROGRESS"
	OperationStatusSuccess    OperationStatus = "SUCCESS"
	OperationStatusFailure    OperationStatus = "FAILURE"
)

// OperationResult represents the result of an operation
type OperationResult struct {
	Status  OperationStatus
	Error   ErrorCode
	Message string
}

// Operation represents a GCP long-running operation
type Operation struct {
	Name   string
	Done   bool
	Error  *OperationError
	Status string
}

// OperationError represents an error from a GCP operation
type OperationError struct {
	Code    int
	Message string
}

// OperationWaiter waits for a GCP operation to complete
type OperationWaiter struct {
	client   *Client
	opName   string
	getURL   string
	timeout  time.Duration
	interval time.Duration
}

// NewOperationWaiter creates a new operation waiter
func NewOperationWaiter(client *Client, opName string, getURL string) *OperationWaiter {
	return &OperationWaiter{
		client:   client,
		opName:   opName,
		getURL:   getURL,
		timeout:  DefaultOperationTimeout,
		interval: DefaultOperationPollInterval,
	}
}

// WithTimeout sets a custom timeout for the waiter
func (w *OperationWaiter) WithTimeout(timeout time.Duration) *OperationWaiter {
	w.timeout = timeout
	return w
}

// WithPollInterval sets a custom poll interval for the waiter
func (w *OperationWaiter) WithPollInterval(interval time.Duration) *OperationWaiter {
	w.interval = interval
	return w
}

// Wait polls the operation until it completes or times out
func (w *OperationWaiter) Wait(ctx context.Context) (*OperationResult, error) {
	deadline := time.Now().Add(w.timeout)

	for {
		// Check timeout
		if time.Now().After(deadline) {
			return &OperationResult{
				Status:  OperationStatusFailure,
				Error:   ErrorCodeTimeout,
				Message: fmt.Sprintf("operation %s timed out after %v", w.opName, w.timeout),
			}, fmt.Errorf("operation timed out")
		}

		// Check context cancellation
		if ctx.Err() != nil {
			return &OperationResult{
				Status:  OperationStatusFailure,
				Error:   ErrorCodeCancelled,
				Message: "operation cancelled",
			}, ctx.Err()
		}

		// Query operation status
		op, err := w.queryOperation(ctx)
		if err != nil {
			// Check if 404 is retryable (eventual consistency)
			// GCP operations may not be immediately visible after creation
			errCode := ClassifyError(err)
			if errCode == ErrorCodeResourceNotFound {
				time.Sleep(w.interval)
				continue
			}

			return &OperationResult{
				Status:  OperationStatusFailure,
				Error:   errCode,
				Message: fmt.Sprintf("failed to query operation: %v", err),
			}, err
		}

		// Check if operation completed
		if op.Done {
			if op.Error != nil {
				return &OperationResult{
					Status:  OperationStatusFailure,
					Error:   ErrorCodeUnknown,
					Message: fmt.Sprintf("operation failed: %s", op.Error.Message),
				}, fmt.Errorf("operation error: code=%d, message=%s", op.Error.Code, op.Error.Message)
			}

			return &OperationResult{
				Status:  OperationStatusSuccess,
				Error:   ErrorCodeNone,
				Message: "operation completed successfully",
			}, nil
		}

		// Operation still in progress, wait and poll again
		select {
		case <-time.After(w.interval):
			// Continue polling
		case <-ctx.Done():
			return &OperationResult{
				Status:  OperationStatusFailure,
				Error:   ErrorCodeCancelled,
				Message: "operation cancelled",
			}, ctx.Err()
		}
	}
}

// queryOperation fetches the current operation status
func (w *OperationWaiter) queryOperation(ctx context.Context) (*Operation, error) {
	resp, err := w.client.SendRequest(ctx, RequestOptions{
		Method:  "GET",
		URL:     w.getURL,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	if resp.Body == nil {
		return nil, fmt.Errorf("empty response body")
	}

	op := &Operation{
		Name: w.opName,
	}

	// Parse operation fields
	if name, ok := resp.Body["name"].(string); ok {
		op.Name = name
	}

	if done, ok := resp.Body["done"].(bool); ok {
		op.Done = done
	}

	if status, ok := resp.Body["status"].(string); ok {
		op.Status = status
	}

	// Parse error if present
	if errObj, ok := resp.Body["error"].(map[string]interface{}); ok {
		op.Error = &OperationError{}
		if code, ok := errObj["code"].(float64); ok {
			op.Error.Code = int(code)
		}
		if msg, ok := errObj["message"].(string); ok {
			op.Error.Message = msg
		}
	}

	return op, nil
}

// ExtractOperationURL extracts the operation URL from a create/update response
// Common patterns:
// - response["name"] = "projects/PROJECT/zones/ZONE/operations/OP_ID"
// - response["selfLink"] = "https://www.googleapis.com/compute/v1/projects/PROJECT/zones/ZONE/operations/OP_ID"
func ExtractOperationURL(response map[string]interface{}, baseURL string) (string, error) {
	// Try selfLink first (full URL)
	if selfLink, ok := response["selfLink"].(string); ok && selfLink != "" {
		return selfLink, nil
	}

	// Try name field (relative path)
	if name, ok := response["name"].(string); ok && name != "" {
		// If name is a full path, construct URL
		if baseURL != "" {
			return fmt.Sprintf("%s/%s", baseURL, name), nil
		}
		return name, nil
	}

	return "", fmt.Errorf("no operation URL found in response")
}

// ParseOperationResponse parses a standard GCP operation response and determines status
func ParseOperationResponse(response map[string]interface{}) (*OperationResult, error) {
	if response == nil {
		return &OperationResult{
			Status:  OperationStatusFailure,
			Error:   ErrorCodeUnknown,
			Message: "empty operation response",
		}, fmt.Errorf("empty response")
	}

	// Check if done field exists
	done, hasDone := response["done"].(bool)

	if !hasDone {
		// No "done" field means this is an inline synchronous response
		// Check for error indicators
		if errorObj, hasError := response["error"].(map[string]interface{}); hasError {
			errorMsg := "unknown error"
			if msg, ok := errorObj["message"].(string); ok {
				errorMsg = msg
			}
			return &OperationResult{
				Status:  OperationStatusFailure,
				Error:   ErrorCodeUnknown,
				Message: errorMsg,
			}, fmt.Errorf("operation error: %s", errorMsg)
		}

		// Assume success if no error
		return &OperationResult{
			Status:  OperationStatusSuccess,
			Error:   ErrorCodeNone,
			Message: "operation completed",
		}, nil
	}

	// Long-running operation
	if !done {
		return &OperationResult{
			Status:  OperationStatusInProgress,
			Error:   ErrorCodeNone,
			Message: "operation in progress",
		}, nil
	}

	// Operation completed - check for errors
	if errorObj, hasError := response["error"].(map[string]interface{}); hasError {
		errorMsg := "unknown error"
		if msg, ok := errorObj["message"].(string); ok {
			errorMsg = msg
		}
		return &OperationResult{
			Status:  OperationStatusFailure,
			Error:   ErrorCodeUnknown,
			Message: errorMsg,
		}, fmt.Errorf("operation error: %s", errorMsg)
	}

	return &OperationResult{
		Status:  OperationStatusSuccess,
		Error:   ErrorCodeNone,
		Message: "operation completed successfully",
	}, nil
}
