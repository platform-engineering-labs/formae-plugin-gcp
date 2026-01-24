// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package status

import (
	"errors"
	"strings"

	"github.com/googleapis/gax-go/v2/apierror"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
)

// MapGCPError maps GCP API errors to Formae error codes
func MapGCPError(err error) resource.OperationErrorCode {
	if err == nil {
		return resource.OperationErrorCodeNotSet
	}

	// Check if it's wrapped in an APIError (newer GCP SDK)
	var apiErr *apierror.APIError
	if errors.As(err, &apiErr) {
		// First try HTTP status code if available
		if apiErr.HTTPCode() > 0 {
			return mapHTTPStatusCode(apiErr.HTTPCode(), strings.ToLower(apiErr.Error()))
		}

		// If no HTTP code, try gRPC status code
		grpcStatus := apiErr.GRPCStatus()
		if grpcStatus != nil {
			return mapGRPCStatusCode(grpcStatus.Code(), strings.ToLower(grpcStatus.Message()))
		}
	}

	// Check if it's a Google API error directly (older GCP SDK)
	var googleErr *googleapi.Error
	if errors.As(err, &googleErr) {
		return mapGoogleAPIError(googleErr)
	}

	// Default to unforeseen error
	return resource.OperationErrorCodeUnforeseenError
}

func mapGoogleAPIError(err *googleapi.Error) resource.OperationErrorCode {
	return mapHTTPStatusCode(err.Code, strings.ToLower(err.Message))
}

func mapHTTPStatusCode(code int, message string) resource.OperationErrorCode {
	// Map by HTTP status code
	switch code {
	case 400:
		// Bad Request
		if strings.Contains(message, "invalid") {
			return resource.OperationErrorCodeInvalidRequest
		}
		if strings.Contains(message, "already exists") {
			return resource.OperationErrorCodeAlreadyExists
		}
		return resource.OperationErrorCodeInvalidRequest

	case 401:
		// Unauthorized
		return resource.OperationErrorCodeInvalidCredentials

	case 403:
		// Forbidden
		return resource.OperationErrorCodeAccessDenied

	case 404:
		// Not Found
		return resource.OperationErrorCodeNotFound

	case 409:
		// Conflict
		if strings.Contains(message, "already exists") {
			return resource.OperationErrorCodeAlreadyExists
		}
		return resource.OperationErrorCodeResourceConflict

	case 429:
		// Too Many Requests - Throttling
		return resource.OperationErrorCodeThrottling

	case 500, 502, 503, 504:
		// Server errors - recoverable
		return resource.OperationErrorCodeServiceInternalError

	default:
		return resource.OperationErrorCodeUnforeseenError
	}
}

func mapGRPCStatusCode(code codes.Code, message string) resource.OperationErrorCode {
	switch code {
	case codes.OK:
		return resource.OperationErrorCodeNotSet

	case codes.Canceled:
		return resource.OperationErrorCodeServiceInternalError

	case codes.Unknown:
		return resource.OperationErrorCodeUnforeseenError

	case codes.InvalidArgument:
		if strings.Contains(message, "already exists") {
			return resource.OperationErrorCodeAlreadyExists
		}
		return resource.OperationErrorCodeInvalidRequest

	case codes.DeadlineExceeded:
		return resource.OperationErrorCodeServiceInternalError

	case codes.NotFound:
		return resource.OperationErrorCodeNotFound

	case codes.AlreadyExists:
		return resource.OperationErrorCodeAlreadyExists

	case codes.PermissionDenied:
		return resource.OperationErrorCodeAccessDenied

	case codes.ResourceExhausted:
		return resource.OperationErrorCodeThrottling

	case codes.FailedPrecondition:
		if strings.Contains(message, "not found") {
			return resource.OperationErrorCodeNotFound
		}
		return resource.OperationErrorCodeInvalidRequest

	case codes.Aborted:
		return resource.OperationErrorCodeResourceConflict

	case codes.OutOfRange:
		return resource.OperationErrorCodeInvalidRequest

	case codes.Unimplemented:
		return resource.OperationErrorCodeServiceInternalError

	case codes.Internal:
		return resource.OperationErrorCodeServiceInternalError

	case codes.Unavailable:
		return resource.OperationErrorCodeServiceInternalError

	case codes.DataLoss:
		return resource.OperationErrorCodeServiceInternalError

	case codes.Unauthenticated:
		return resource.OperationErrorCodeInvalidCredentials

	default:
		return resource.OperationErrorCodeUnforeseenError
	}
}
