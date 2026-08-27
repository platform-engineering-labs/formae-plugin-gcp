// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package sql

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// userProvisioner overrides the one operation the generic engine cannot express
// for Cloud SQL users, and delegates the rest. List is handled for every
// instance-scoped type by instance_walking_list.go.
//
// Users are addressed inconsistently by this API: `get` takes the name as a
// path segment, but `delete` takes it as a *query parameter* against the
// collection URL - "DELETE .../users?name=x". The generic engine builds
// ".../users/{name}", which addresses nothing.
type userProvisioner struct {
	prov.Provisioner
	cfg *config.Config
}

// registerUserOverrides is called from the package init in resources.go rather
// than from an init of its own, so it cannot matter whether this file sorts
// before or after resources.go - the generic registration must land first, or
// there would be no provisioner to wrap.
func registerUserOverrides() {
	registry.Register(UserResourceType,
		[]resource.Operation{resource.OperationDelete},
		func(cfg *config.Config) prov.Provisioner {
			return &userProvisioner{
				Provisioner: sqlRegistry.CreateProvisioner(cfg, UserResourceType),
				cfg:         cfg,
			}
		})
}

// collectionURL builds ".../instances/{instance}/users" plus the ?name=&host=
// pair that identifies one user.
func (p *userProvisioner) collectionURL(nativeID string, withIdentity bool) (string, error) {
	ctx, err := parseSQLNativeID(nativeID)
	if err != nil {
		return "", err
	}
	if ctx.ParentType != "instances" || ctx.ParentResource == "" || ctx.ResourceName == "" {
		return "", fmt.Errorf("invalid SQL user native ID: %s", nativeID)
	}
	url := fmt.Sprintf("%s/projects/%s/instances/%s/users",
		SQLAPI.BaseURL, ctx.Project, ctx.ParentResource)
	if !withIdentity {
		return url, nil
	}
	return transport.AddQueryParam(url, "name", ctx.ResourceName)
}

func (p *userProvisioner) Delete(
	ctx context.Context, request *resource.DeleteRequest,
) (*resource.DeleteResult, error) {
	url, err := p.collectionURL(request.NativeID, true)
	if err != nil {
		return p.failedDelete(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	client, err := transport.NewClient(ctx, p.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	response, err := client.SendRequest(ctx, transport.RequestOptions{Method: "DELETE", URL: url})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to delete SQL user")
		code := transport.ToResourceErrorCode(wrapped.Code)
		// A user that is already gone is a delete that already happened.
		if code == resource.OperationErrorCodeNotFound {
			return &resource.DeleteResult{ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusSuccess,
				NativeID:        request.NativeID,
				StatusMessage:   "user already deleted",
			}}, nil
		}
		return p.failedDelete(request.NativeID, code, wrapped.Message), nil
	}

	// Like every other sqladmin mutation, the answer is an Operation to poll.
	return &resource.DeleteResult{ProgressResult: &resource.ProgressResult{
		Operation:       resource.OperationDelete,
		OperationStatus: resource.OperationStatusInProgress,
		NativeID:        request.NativeID,
		RequestID:       p.operationRequestID(response.Body, request.NativeID),
		StatusMessage:   "user deletion in progress",
	}}, nil
}

// operationRequestID turns the Operation the API answers with into the path
// base.Status polls, matching what SQLOperations does for every other type.
func (p *userProvisioner) operationRequestID(body map[string]interface{}, nativeID string) string {
	opID, _ := body["name"].(string)
	if opID == "" {
		return ""
	}
	ctx, err := parseSQLNativeID(nativeID)
	if err != nil {
		return ""
	}
	return SQLOperations.OperationURLBuilder(ctx, opID)
}

func (p *userProvisioner) failedDelete(
	nativeID string, code resource.OperationErrorCode, message string,
) *resource.DeleteResult {
	return &resource.DeleteResult{ProgressResult: &resource.ProgressResult{
		Operation:       resource.OperationDelete,
		OperationStatus: resource.OperationStatusFailure,
		ErrorCode:       code,
		NativeID:        nativeID,
		StatusMessage:   message,
	}}
}
