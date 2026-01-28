// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package transport

import (
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// ToResourceErrorCode converts transport ErrorCode to resource.OperationErrorCode
func ToResourceErrorCode(code ErrorCode) resource.OperationErrorCode {
	switch code {
	case ErrorCodeNone:
		return resource.OperationErrorCodeNotSet
	case ErrorCodeInvalidInput:
		return resource.OperationErrorCodeInvalidRequest
	case ErrorCodeUnauthorized:
		return resource.OperationErrorCodeAccessDenied
	case ErrorCodeResourceNotFound:
		return resource.OperationErrorCodeNotFound
	case ErrorCodeAlreadyExists:
		return resource.OperationErrorCodeAlreadyExists
	case ErrorCodeConcurrencyConflict:
		return resource.OperationErrorCodeResourceConflict
	case ErrorCodeThrottling:
		return resource.OperationErrorCodeThrottling
	case ErrorCodeInternalError:
		return resource.OperationErrorCodeServiceInternalError
	case ErrorCodeTimeout:
		return resource.OperationErrorCodeServiceTimeout
	case ErrorCodeCancelled:
		return resource.OperationErrorCodeUnforeseenError
	case ErrorCodeNotStabilized:
		return resource.OperationErrorCodeNotStabilized
	case ErrorCodeUnknown:
		return resource.OperationErrorCodeUnforeseenError
	default:
		return resource.OperationErrorCodeUnforeseenError
	}
}

// ToResourceOperationStatus converts transport OperationStatus to resource.OperationStatus
func ToResourceOperationStatus(status OperationStatus) resource.OperationStatus {
	switch status {
	case OperationStatusPending:
		return resource.OperationStatusPending
	case OperationStatusInProgress:
		return resource.OperationStatusInProgress
	case OperationStatusSuccess:
		return resource.OperationStatusSuccess
	case OperationStatusFailure:
		return resource.OperationStatusFailure
	default:
		return resource.OperationStatusFailure
	}
}

// ToResourceProgressResult converts OperationResult to resource.ProgressResult
func ToResourceProgressResult(result *OperationResult, operation resource.Operation) *resource.ProgressResult {
	if result == nil {
		return &resource.ProgressResult{
			Operation:       operation,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       resource.OperationErrorCodeUnforeseenError,
			StatusMessage:   "nil operation result",
		}
	}

	return &resource.ProgressResult{
		Operation:       operation,
		OperationStatus: ToResourceOperationStatus(result.Status),
		ErrorCode:       ToResourceErrorCode(result.Error),
		StatusMessage:   result.Message,
	}
}

// FromResourceErrorCode converts resource.OperationErrorCode to transport ErrorCode
func FromResourceErrorCode(code resource.OperationErrorCode) ErrorCode {
	switch code {
	case resource.OperationErrorCodeNotSet:
		return ErrorCodeNone
	case resource.OperationErrorCodeInvalidRequest:
		return ErrorCodeInvalidInput
	case resource.OperationErrorCodeAccessDenied:
		return ErrorCodeUnauthorized
	case resource.OperationErrorCodeNotFound:
		return ErrorCodeResourceNotFound
	case resource.OperationErrorCodeAlreadyExists:
		return ErrorCodeAlreadyExists
	case resource.OperationErrorCodeResourceConflict:
		return ErrorCodeConcurrencyConflict
	case resource.OperationErrorCodeThrottling:
		return ErrorCodeThrottling
	case resource.OperationErrorCodeServiceInternalError:
		return ErrorCodeInternalError
	case resource.OperationErrorCodeServiceTimeout:
		return ErrorCodeTimeout
	case resource.OperationErrorCodeNotStabilized:
		return ErrorCodeNotStabilized
	case resource.OperationErrorCodeUnforeseenError:
		return ErrorCodeUnknown
	default:
		return ErrorCodeUnknown
	}
}
