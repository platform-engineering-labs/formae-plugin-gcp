// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package base

// OperationConfig defines how operations work for a GCP API
type OperationConfig struct {
	// Synchronous indicates if operations complete immediately (true)
	// or require polling (false). Most GCP APIs are asynchronous.
	Synchronous bool

	// OperationIDExtractor extracts the operation ID from create/update/delete response
	// For async APIs, this is used to poll for operation status
	OperationIDExtractor func(response map[string]interface{}) string

	// OperationURLBuilder constructs the URL to check operation status
	// Takes the path context and operation ID extracted from response
	OperationURLBuilder func(ctx PathContext, operationID string) string

	// NativeIDExtractor extracts the native ID from response
	// Different APIs return IDs in different formats
	NativeIDExtractor func(response map[string]interface{}, ctx PathContext) string

	// OperationStatusChecker checks if an operation is complete and successful
	// Returns (done=true, err=nil) on success
	// Returns (done=true, err=error) on failure
	// Returns (done=false, err=nil) if still in progress
	OperationStatusChecker func(operationResponse map[string]interface{}) (done bool, err error)
}

// OperationResult represents the result of checking operation status
type OperationResult struct {
	Done    bool
	Error   error
	Message string
}
