// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package certificateauthority

import (
	"context"
	"encoding/json"
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

// Read reports a certificate authority in state DELETED as gone.
//
// skipGracePeriod asks for immediate destruction, but the CA does not vanish
// from the API the moment the delete operation finishes: it first moves to
// state DELETED, and a GET keeps answering with the resource. To formae that
// reads as "still there", so a destroy never settles and an out-of-band delete
// is never noticed - the resource sits in inventory until the check times out.
//
// The same tombstone shows up on Cloud SQL backup runs (status DELETED) and
// Logging buckets (lifecycleState DELETE_REQUESTED). A deleted thing is not
// found, so say so.
func (c *caProvisioner) Read(
	ctx context.Context,
	request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	result, err := c.BaseResource.Read(ctx, request)
	if err != nil || result == nil || result.Properties == "" {
		return result, err
	}

	var props map[string]interface{}
	if unmarshalErr := json.Unmarshal([]byte(result.Properties), &props); unmarshalErr != nil {
		return result, nil
	}
	if isDeletedTombstone(props) {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
	}
	return result, nil
}

// isDeletedTombstone reports whether these properties describe a CA that has
// already been deleted and is only still answering GETs.
func isDeletedTombstone(props map[string]interface{}) bool {
	state, ok := props["state"].(string)
	return ok && state == "DELETED"
}
