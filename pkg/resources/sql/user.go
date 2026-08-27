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

// userProvisioner overrides the two operations the generic engine cannot
// express for Cloud SQL users, and delegates create, read and check-status.
//
// Users are addressed inconsistently by this API: `get` takes the name as a
// path segment, but `delete` takes it as a *query parameter* against the
// collection URL - "DELETE .../users?name=x". The generic engine builds
// ".../users/{name}", which addresses nothing.
//
// List is overridden for the usual reason: discovery lists with no properties,
// so it can name no instance to look in, and sqladmin has no wildcard.
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
		[]resource.Operation{
			resource.OperationDelete,
			resource.OperationList,
		},
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

// List walks the instances, because discovery names none and sqladmin has no
// wildcard for them.
func (p *userProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	if request.AdditionalProperties != nil && request.AdditionalProperties["instance"] != "" {
		return p.Provisioner.List(ctx, request)
	}

	cfg := config.PathFromTargetConfig(request.TargetConfig)
	if cfg.Project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, p.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	instancesURL := fmt.Sprintf("%s/projects/%s/instances", SQLAPI.BaseURL, cfg.Project)
	resp, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: instancesURL})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to list SQL instances")
		return nil, fmt.Errorf("%s", wrapped.Message)
	}

	nativeIDs := []string{}
	instances, _ := resp.Body["items"].([]interface{})
	for _, raw := range instances {
		inst, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		instName, _ := inst["name"].(string)
		if instName == "" {
			continue
		}
		usersResp, listErr := client.SendRequest(ctx, transport.RequestOptions{
			Method: "GET",
			URL:    fmt.Sprintf("%s/%s/users", instancesURL, instName),
		})
		if listErr != nil {
			// One unreadable instance must not hide the rest.
			continue
		}
		users, _ := usersResp.Body["items"].([]interface{})
		for _, rawUser := range users {
			user, ok := rawUser.(map[string]interface{})
			if !ok {
				continue
			}
			if name, _ := user["name"].(string); name != "" {
				nativeIDs = append(nativeIDs,
					fmt.Sprintf("projects/%s/instances/%s/users/%s", cfg.Project, instName, name))
			}
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
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
