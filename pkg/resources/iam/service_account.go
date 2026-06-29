// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package iam

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
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
			Config:          cfg,
			APIConfig:       IAMAPI,
			OperationConfig: IAMOperations,
			ResourceConfig:  base.ResourceConfig{ResourceType: "serviceAccounts", SupportsUpdate: false},
			NativeIDConfig:  IAMNativeID,
		},
	}
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
	propsJSON, _ := json.Marshal(response.Body)
	return createSuccess(nativeID, propsJSON), nil
}
