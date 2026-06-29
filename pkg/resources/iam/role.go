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

const RoleResourceType = "GCP::IAM::Role"

// roleMutableFields are the fields a custom-role PATCH may modify (name/roleId
// is createOnly).
var roleMutableFields = []string{"title", "description", "includedPermissions", "stage"}

// RoleProvisioner manages IAM custom (project-level) roles. Create POSTs to the
// collection with ?roleId=<name> and the role wrapped under "role". Read/Delete
// use the base engine. Update uses the base engine with UpdateMaskFromBody (the
// roles.patch body is the Role directly + ?updateMask).
type RoleProvisioner struct {
	*base.BaseResource
}

func init() {
	registry.Register(
		RoleResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		newRoleProvisioner,
	)
}

func newRoleProvisioner(cfg *config.Config) prov.Provisioner {
	return &RoleProvisioner{
		BaseResource: &base.BaseResource{
			Config:          cfg,
			APIConfig:       IAMAPI,
			OperationConfig: IAMOperations,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "roles",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			NativeIDConfig:     IAMNativeID,
			RequestTransformer: base.RequestTransformerFunc(roleUpdateTransformer),
		},
	}
}

// roleUpdateTransformer keeps only mutable role fields so the base update path's
// updateMask never names an immutable field. (Create is handled by the override
// below, so this only runs for updates.)
func roleUpdateTransformer(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
	return pickRoleFields(props), nil
}

// pickRoleFields returns the subset of props that are valid role body fields.
func pickRoleFields(props map[string]interface{}) map[string]interface{} {
	body := make(map[string]interface{})
	for _, k := range roleMutableFields {
		if v, ok := props[k]; ok {
			body[k] = v
		}
	}
	return body
}

// Create POSTs to the collection with ?roleId=<name> and {"role": {...}}.
func (p *RoleProvisioner) Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error) {
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	var props map[string]interface{}
	if err := json.Unmarshal(req.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest, fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	name := utils.GetString(props, "name") // the roleId
	if name == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest, "name (roleId) is required"), nil
	}

	cfg := config.FromTargetConfig(req.TargetConfig)
	pathCtx := base.PathContext{Project: cfg.Project, ResourceType: "roles"}

	body := map[string]interface{}{"role": pickRoleFields(props)}

	url, err := transport.AddQueryParams(base.NewURLBuilder(IAMAPI, pathCtx).CollectionURL(), map[string]string{"roleId": name})
	if err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest, fmt.Sprintf("failed to build URL: %v", err)), nil
	}

	response, err := client.SendRequest(ctx, transport.RequestOptions{Method: "POST", URL: url, Body: body})
	if err != nil {
		transportErr := transport.WrapError(err, fmt.Sprintf("failed to create role '%s'", name))
		return createFailure(transport.ToResourceErrorCode(transportErr.Code), transportErr.Message), nil
	}

	nativeID := extractIAMNativeID(response.Body, pathCtx)
	propsJSON, _ := json.Marshal(response.Body)
	return createSuccess(nativeID, propsJSON), nil
}
