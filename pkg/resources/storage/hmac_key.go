// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package storage

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// hmacKeyDeactivatedState is the only state GCS will delete a key from. An
// active key answers a delete with 400: it has to be deactivated first, which
// is a separate PUT.
const hmacKeyDeactivatedState = "INACTIVE"

// hmacKeyRequestTransformer builds the create body. A key is created by naming
// the service account it belongs to in a query parameter, not in a body - the
// request carries no body at all - so this drops everything and lets
// CreateIDParam-style handling happen in the provisioner.
func hmacKeyRequestTransformer(
	props map[string]interface{}, _ base.TransformContext,
) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

// hmacKeyResponseTransformer lifts the metadata out of the create envelope and
// drops the secret.
//
// create answers {"metadata": {...}, "secret": "..."} while get answers the
// metadata directly, so reads would otherwise disagree with creates on every
// field. The secret is returned exactly once and never again: keeping it would
// both persist a credential and guarantee drift on the next read.
func hmacKeyResponseTransformer(
	props map[string]interface{}, _ base.TransformContext,
) map[string]interface{} {
	metadata := props
	if nested, ok := props["metadata"].(map[string]interface{}); ok {
		metadata = nested
	}

	out := make(map[string]interface{}, len(metadata))
	for k, v := range metadata {
		switch k {
		case "secret", "kind", "selfLink", "etag", "id":
			continue
		}
		out[k] = v
	}
	return out
}

// hmacKeyProvisioner overrides the two operations the generic engine cannot
// express: a create whose id is a query parameter and whose response wraps the
// resource, and a delete that must deactivate the key first.
type hmacKeyProvisioner struct {
	prov.Provisioner
	cfg *config.Config
}

// registerHmacKeyOverrides is called from the package init in resources.go so
// the generic registration is guaranteed to have landed first.
func registerHmacKeyOverrides() {
	registry.Register(HmacKeyResourceType,
		[]resource.Operation{resource.OperationCreate, resource.OperationDelete},
		func(cfg *config.Config) prov.Provisioner {
			return &hmacKeyProvisioner{
				Provisioner: storageRegistry.CreateProvisioner(cfg, HmacKeyResourceType),
				cfg:         cfg,
			}
		})
}

func (p *hmacKeyProvisioner) Create(
	ctx context.Context, request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	props, err := unmarshalProps(request.Properties)
	if err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	serviceAccountEmail := utils.GetString(props, "serviceAccountEmail")
	if serviceAccountEmail == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"serviceAccountEmail is required to create an HMAC key"), nil
	}

	cfg := config.PathFromTargetConfig(request.TargetConfig)
	client, err := transport.NewClient(ctx, p.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	url := fmt.Sprintf("%s/projects/%s/hmacKeys?serviceAccountEmail=%s",
		StorageAPI.BaseURL, cfg.Project, serviceAccountEmail)
	response, err := client.SendRequest(ctx, transport.RequestOptions{Method: "POST", URL: url})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to create HMAC key")
		return createFailure(transport.ToResourceErrorCode(wrapped.Code), wrapped.Message), nil
	}

	accessID := hmacKeyAccessID(response.Body)
	if accessID == "" {
		return createFailure(resource.OperationErrorCodeServiceInternalError,
			"HMAC key create returned no accessId"), nil
	}

	properties := hmacKeyResponseTransformer(response.Body, base.TransformContext{})
	return &resource.CreateResult{ProgressResult: &resource.ProgressResult{
		Operation:          resource.OperationCreate,
		OperationStatus:    resource.OperationStatusSuccess,
		NativeID:           fmt.Sprintf("projects/%s/hmacKeys/%s", cfg.Project, accessID),
		StatusMessage:      "Resource created successfully",
		ResourceProperties: mustMarshal(properties),
	}}, nil
}

// Delete deactivates the key before removing it. GCS refuses to delete a key
// that is still ACTIVE, so a single DELETE is never enough.
func (p *hmacKeyProvisioner) Delete(
	ctx context.Context, request *resource.DeleteRequest,
) (*resource.DeleteResult, error) {
	pathCtx, err := parseStorageNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(request.NativeID, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	cfg := config.PathFromTargetConfig(request.TargetConfig)
	project := pathCtx.Project
	if project == "" {
		project = cfg.Project
	}

	client, err := transport.NewClient(ctx, p.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}
	url := fmt.Sprintf("%s/projects/%s/hmacKeys/%s", StorageAPI.BaseURL, project, pathCtx.ResourceName)

	// Read first: the update needs the key's current etag-free metadata, and a
	// key already gone is a delete that already happened.
	current, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: url})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to read HMAC key before deleting")
		if transport.ToResourceErrorCode(wrapped.Code) == resource.OperationErrorCodeNotFound {
			return deleteSuccess(request.NativeID, "HMAC key already deleted"), nil
		}
		return deleteFailure(request.NativeID, transport.ToResourceErrorCode(wrapped.Code), wrapped.Message), nil
	}

	if utils.GetString(current.Body, "state") != hmacKeyDeactivatedState {
		body := map[string]interface{}{
			"state":               hmacKeyDeactivatedState,
			"accessId":            pathCtx.ResourceName,
			"serviceAccountEmail": utils.GetString(current.Body, "serviceAccountEmail"),
		}
		if _, err := client.SendRequest(ctx, transport.RequestOptions{
			Method: "PUT", URL: url, Body: body,
		}); err != nil {
			wrapped := transport.WrapError(err, "failed to deactivate HMAC key before deleting")
			return deleteFailure(request.NativeID,
				transport.ToResourceErrorCode(wrapped.Code), wrapped.Message), nil
		}
	}

	if _, err := client.SendRequest(ctx, transport.RequestOptions{Method: "DELETE", URL: url}); err != nil {
		wrapped := transport.WrapError(err, "failed to delete HMAC key")
		if transport.ToResourceErrorCode(wrapped.Code) == resource.OperationErrorCodeNotFound {
			return deleteSuccess(request.NativeID, "HMAC key already deleted"), nil
		}
		return deleteFailure(request.NativeID, transport.ToResourceErrorCode(wrapped.Code), wrapped.Message), nil
	}
	return deleteSuccess(request.NativeID, "Resource deleted successfully"), nil
}

// hmacKeyAccessID reads the key id from either shape: create wraps the metadata,
// get returns it directly.
func hmacKeyAccessID(response map[string]interface{}) string {
	if metadata, ok := response["metadata"].(map[string]interface{}); ok {
		if id := utils.GetString(metadata, "accessId"); id != "" {
			return id
		}
	}
	return utils.GetString(response, "accessId")
}
