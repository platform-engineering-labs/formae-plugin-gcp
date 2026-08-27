// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package iam

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const ServiceAccountResourceType = "GCP::IAM::ServiceAccount"

// deleteStatusPrefix marks a Status RequestID as belonging to the delete flow
// (create uses the bare NativeID). See Status for why the two are distinguished.
const deleteStatusPrefix = "delete:"

// ServiceAccountProvisioner manages IAM service accounts. Read/List use the
// generic base engine off the full resource path
// ("projects/{p}/serviceAccounts/{email}"). Create is overridden because the
// create body nests the account under "serviceAccount" alongside an "accountId"
// sibling, and it returns InProgress. Delete delegates the DELETE to the base
// engine but also returns InProgress. Both Create and Delete rely on Status to
// wait out IAM eventual consistency in opposite directions (see below). Update
// (displayName/description) is deferred - it uses a body-wrapped PATCH that
// needs its own override.
type ServiceAccountProvisioner struct {
	*base.BaseResource
}

func init() {
	registry.Register(
		ServiceAccountResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		newServiceAccountProvisioner,
	)
}

func newServiceAccountProvisioner(cfg *config.Config) prov.Provisioner {
	return &ServiceAccountProvisioner{
		BaseResource: &base.BaseResource{
			Config:              cfg,
			APIConfig:           IAMAPI,
			OperationConfig:     IAMOperations,
			ResourceConfig:      base.ResourceConfig{ResourceType: "serviceAccounts", SupportsUpdate: false, ListItemsKey: "accounts"},
			NativeIDConfig:      IAMNativeID,
			ResponseTransformer: base.ResponseTransformerFunc(serviceAccountResponseTransformer),
		},
	}
}

// serviceAccountResponseTransformer normalizes the "name" property to the
// declared accountId. The API returns name="projects/{p}/serviceAccounts/{email}"
// and email="{accountId}@{project}.iam.gserviceaccount.com"; the accountId is the
// local part of the email.
func serviceAccountResponseTransformer(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
	if email, ok := apiResponse["email"].(string); ok {
		if at := strings.Index(email, "@"); at > 0 {
			apiResponse["name"] = email[:at]
		}
	}
	return apiResponse
}

// Create POSTs to the collection with {"accountId": <name>, "serviceAccount": {...}}.
func (p *ServiceAccountProvisioner) Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error) {
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	var props map[string]interface{}
	if err := json.Unmarshal(req.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest, fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	name := utils.GetString(props, "name") // the accountId
	if name == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest, "name (accountId) is required"), nil
	}

	cfg := config.FromTargetConfig(req.TargetConfig, p.Config.Deps())
	pathCtx := base.PathContext{Project: cfg.Project, ResourceType: "serviceAccounts"}

	sa := make(map[string]interface{})
	if v := utils.GetString(props, "displayName"); v != "" {
		sa["displayName"] = v
	}
	if v := utils.GetString(props, "description"); v != "" {
		sa["description"] = v
	}
	body := map[string]interface{}{"accountId": name, "serviceAccount": sa}

	url := base.NewURLBuilder(IAMAPI, pathCtx).CollectionURL()
	response, err := client.SendRequest(ctx, transport.RequestOptions{Method: "POST", URL: url, Body: body})
	if err != nil {
		transportErr := transport.WrapError(err, fmt.Sprintf("failed to create service account '%s'", name))
		return createFailure(transport.ToResourceErrorCode(transportErr.Code), transportErr.Message), nil
	}

	nativeID := extractIAMNativeID(response.Body, pathCtx)
	if nativeID == "" {
		return createFailure(resource.OperationErrorCodeServiceInternalError,
			"service account create response had no resource name"), nil
	}

	// A freshly created service account is eventually consistent: it does not
	// appear in serviceAccounts.list for a short window after create. Return
	// InProgress and let Status poll until it is listable, so the create only
	// completes once a subsequent sync/inventory is guaranteed to find it.
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        nativeID,
			RequestID:       nativeID,
			StatusMessage:   "service account created; waiting for read consistency",
		},
	}, nil
}

// Delete removes the service account and returns InProgress, delegating the
// actual DELETE (and 404-already-gone handling) to the base engine. Deletion is
// eventually consistent the same way create is: the account keeps showing up in
// serviceAccounts.list for a window after the DELETE returns. A synchronous
// Success here lets a sync in that window re-observe the account and fail to
// tombstone it (the OOB-delete inventory-removal flake). Returning InProgress
// makes Status poll until the account is gone from list, mirroring Create.
func (p *ServiceAccountProvisioner) Delete(ctx context.Context, req *resource.DeleteRequest) (*resource.DeleteResult, error) {
	res, err := p.BaseResource.Delete(ctx, req)
	if err != nil {
		return nil, err
	}
	// Propagate failures unchanged; only a successful DELETE flips to InProgress.
	if res.ProgressResult == nil || res.ProgressResult.OperationStatus != resource.OperationStatusSuccess {
		return res, nil
	}
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        req.NativeID,
			RequestID:       deleteStatusPrefix + req.NativeID,
			StatusMessage:   "service account deleted; waiting for it to disappear from list",
		},
	}, nil
}

// deleteStatus is InProgress until the service account no longer appears in
// serviceAccounts.list — the inverse of the create consistency check. Once the
// account is gone from list, a subsequent sync/inventory is guaranteed to
// tombstone it, so the delete completes.
func (p *ServiceAccountProvisioner) deleteStatus(ctx context.Context, req *resource.StatusRequest) (*resource.StatusResult, error) {
	listed, err := p.isListed(ctx, req)
	if err != nil {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeServiceInternalError,
				NativeID:        req.NativeID,
				StatusMessage:   fmt.Sprintf("service account list failed during delete consistency check: %v", err),
			},
		}, nil
	}
	if !listed {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusSuccess,
				NativeID:        req.NativeID,
				StatusMessage:   "service account no longer listable",
			},
		}, nil
	}
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        req.NativeID,
			RequestID:       req.RequestID,
			StatusMessage:   "waiting for service account to disappear from list",
		},
	}, nil
}

// Status confirms the service account has become eventually consistent. For the
// create flow it is InProgress until the account shows up in
// serviceAccounts.list, which is the same read path a force-sync / inventory
// uses (the create otherwise completes before the SA is listable, and a sync in
// that window drops it → the "got 0" flake). Once listed, it reads the account
// back for the final properties. The delete flow (RequestID carries
// deleteStatusPrefix) is the inverse and routes to deleteStatus.
func (p *ServiceAccountProvisioner) Status(ctx context.Context, req *resource.StatusRequest) (*resource.StatusResult, error) {
	if strings.HasPrefix(req.RequestID, deleteStatusPrefix) {
		return p.deleteStatus(ctx, req)
	}

	listed, err := p.isListed(ctx, req)
	if err != nil {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeServiceInternalError,
				NativeID:        req.NativeID,
				StatusMessage:   fmt.Sprintf("service account list failed during consistency check: %v", err),
			},
		}, nil
	}

	if listed {
		// Listable — read back for the canonical properties.
		if read, rerr := p.Read(ctx, &resource.ReadRequest{
			NativeID: req.NativeID, ResourceType: req.ResourceType, TargetConfig: req.TargetConfig,
		}); rerr == nil && read.ErrorCode == "" {
			return &resource.StatusResult{
				ProgressResult: &resource.ProgressResult{
					Operation:          resource.OperationCheckStatus,
					OperationStatus:    resource.OperationStatusSuccess,
					NativeID:           req.NativeID,
					ResourceProperties: []byte(read.Properties),
					StatusMessage:      "service account is listable",
				},
			}, nil
		}
		// Listed but not yet readable — keep waiting for full consistency.
	}

	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        req.NativeID,
			RequestID:       req.RequestID,
			StatusMessage:   "waiting for service account to appear in list",
		},
	}, nil
}

// isListed reports whether req.NativeID appears in serviceAccounts.list,
// following pagination.
func (p *ServiceAccountProvisioner) isListed(ctx context.Context, req *resource.StatusRequest) (bool, error) {
	var pageToken *string
	for {
		res, err := p.List(ctx, &resource.ListRequest{
			ResourceType: req.ResourceType,
			TargetConfig: req.TargetConfig,
			PageToken:    pageToken,
		})
		if err != nil {
			return false, err
		}
		for _, id := range res.NativeIDs {
			if id == req.NativeID {
				return true, nil
			}
		}
		if res.NextPageToken == nil || *res.NextPageToken == "" {
			return false, nil
		}
		pageToken = res.NextPageToken
	}
}
