// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package certificateauthority

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// caProvisioner exists for one reason: a plain DELETE of a certificate
// authority does not delete it.
//
// By default the CA moves to state DELETED and sits there for a 30 day grace
// period, still holding its id and still counting as a CA. Anyone who meant
// "delete" would see it linger, and the conformance out-of-band delete check
// would find a tombstone where it expected nothing - the same shape as the
// Cloud SQL backup run and the Logging bucket. Here the API offers a way out:
// skipGracePeriod destroys it for real.
//
// ignoreActiveCertificates and ignoreDependentResources go along, so a CA that
// did issue something still tears down instead of blocking the delete.
type caProvisioner struct {
	*base.BaseResource
}

var caDeleteParams = map[string]string{
	"skipGracePeriod":          "true",
	"ignoreActiveCertificates": "true",
	"ignoreDependentResources": "true",
}

func (c *caProvisioner) Delete(
	ctx context.Context,
	request *resource.DeleteRequest,
) (*resource.DeleteResult, error) {
	client, err := transport.NewClient(ctx, c.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	nativeID := request.NativeID
	pathCtx, err := base.ParseNativeID(c.NativeIDConfig, nativeID)
	if err != nil {
		return c.deleteFailure(nativeID, resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid native ID: %v", err)), nil
	}

	url, err := transport.AddQueryParams(c.APIConfig.BaseURL+"/"+nativeID, caDeleteParams)
	if err != nil {
		return c.deleteFailure(nativeID, resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to build URL: %v", err)), nil
	}

	response, err := client.SendRequest(ctx, transport.RequestOptions{Method: "DELETE", URL: url})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to delete certificate authority")
		code := transport.ToResourceErrorCode(wrapped.Code)
		if code == resource.OperationErrorCodeNotFound {
			return c.deleteProgress(nativeID, resource.OperationStatusSuccess,
				"certificateAuthorities already deleted", ""), nil
		}
		return c.deleteFailure(nativeID, code, wrapped.Message), nil
	}

	operationID := c.OperationConfig.OperationIDExtractor(response.Body)
	if operationID == "" {
		return c.deleteProgress(nativeID, resource.OperationStatusSuccess,
			"Resource deleted successfully", ""), nil
	}

	return c.deleteProgress(nativeID, resource.OperationStatusInProgress,
		"certificateAuthorities deletion in progress",
		c.OperationConfig.OperationURLBuilder(pathCtx, operationID)), nil
}

func (c *caProvisioner) deleteProgress(
	nativeID string,
	status resource.OperationStatus,
	message, requestID string,
) *resource.DeleteResult {
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: status,
			NativeID:        nativeID,
			RequestID:       requestID,
			StatusMessage:   message,
		},
	}
}

func (c *caProvisioner) deleteFailure(
	nativeID string,
	code resource.OperationErrorCode,
	message string,
) *resource.DeleteResult {
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusFailure,
			NativeID:        nativeID,
			StatusMessage:   message,
			ErrorCode:       code,
		},
	}
}
