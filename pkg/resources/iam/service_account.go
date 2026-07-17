// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package iam

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// saReadPropagationAttempts / saReadPropagationDelay bound how long Read tolerates
// a post-create 404. A freshly created IAM service account is not immediately
// readable (eventual consistency): serviceAccounts.get can 404 for a few seconds.
// A force-sync in that window would read NotFound and drop the SA from inventory
// (observed as the intermittent conformance "got 0 after sync" flake). Retrying
// the read briefly absorbs the lag. Genuinely-absent SAs still resolve to
// NotFound, just after the bounded wait.
const (
	saReadPropagationAttempts = 6
	saReadPropagationDelay    = 2 * time.Second
)

const ServiceAccountResourceType = "GCP::IAM::ServiceAccount"

// ServiceAccountProvisioner manages IAM service accounts. Read/Delete/List use
// the generic base engine off the full resource path
// ("projects/{p}/serviceAccounts/{email}"); only Create needs an override
// because the create body nests the account under "serviceAccount" alongside an
// "accountId" sibling. Update (displayName/description) is deferred - it uses a
// body-wrapped PATCH that needs its own override.
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

// Read wraps the base read to tolerate IAM eventual consistency: a service
// account can 404 for a few seconds after creation, so a NotFound is retried
// briefly before being reported. This prevents a force-sync soon after create
// from dropping the SA from inventory.
func (p *ServiceAccountProvisioner) Read(ctx context.Context, req *resource.ReadRequest) (*resource.ReadResult, error) {
	var res *resource.ReadResult
	var err error
	for attempt := 0; attempt < saReadPropagationAttempts; attempt++ {
		res, err = p.BaseResource.Read(ctx, req)
		if err != nil || res.ErrorCode != resource.OperationErrorCodeNotFound {
			return res, err
		}
		if attempt == saReadPropagationAttempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return res, err
		case <-time.After(saReadPropagationDelay):
		}
	}
	return res, err
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

	cfg := config.FromTargetConfig(req.TargetConfig)
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
	respBody := p.ResponseTransformer.Transform(response.Body, base.TransformContext{
		Project:      pathCtx.Project,
		ResourceType: pathCtx.ResourceType,
		Operation:    resource.OperationCreate,
	})
	propsJSON, _ := json.Marshal(respBody)
	return createSuccess(nativeID, propsJSON), nil
}
