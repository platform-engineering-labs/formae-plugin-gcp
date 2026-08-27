// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package secretmanager

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

const SecretResourceType = "GCP::SecretManager::Secret"

var secretManagerRegistry *base.ResourceRegistry

// SecretProvisioner wraps BaseResource to handle Secret Manager's create
// convention: POST to the collection with a ?secretId=<name> query parameter.
// Read/Delete/List are inherited from BaseResource (they operate on the full
// resource path natively).
type SecretProvisioner struct {
	*base.BaseResource
}

func init() {
	secretManagerRegistry = base.NewResourceRegistry(SecretManagerAPI, SecretManagerOperations, SecretManagerNativeID)

	err := secretManagerRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: SecretResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "secrets",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true, // PATCH ?updateMask=<body fields>
			},
			RequestTransformer:  base.RequestTransformerFunc(secretRequestTransformer),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
	})
	if err != nil {
		panic(err)
	}

	registry.Register(
		SecretResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(cfg *config.Config) prov.Provisioner {
			def := secretManagerRegistry.Definitions[SecretResourceType]
			baseResource := &base.BaseResource{
				Config:              cfg,
				APIConfig:           SecretManagerAPI,
				OperationConfig:     SecretManagerOperations,
				ResourceConfig:      def.ResourceConfig,
				NativeIDConfig:      SecretManagerNativeID,
				RequestTransformer:  def.RequestTransformer,
				ResponseTransformer: def.ResponseTransformer,
			}
			return &SecretProvisioner{BaseResource: baseResource}
		},
	)
}

// secretMutableFields are the fields a Secret PATCH may modify. replication and
// name are createOnly and must not appear in an update body / updateMask.
var secretMutableFields = map[string]bool{"labels": true}

// secretRequestTransformer shapes the request body per operation:
//   - Create: drop "name" (carried in ?secretId); keep replication + labels.
//   - Update: keep only mutable fields so updateMask never names an immutable
//     field (which Secret Manager would reject).
func secretRequestTransformer(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		if k == "name" {
			continue
		}
		if ctx.Operation == resource.OperationUpdate && !secretMutableFields[k] {
			continue
		}
		body[k] = v
	}

	// On create, the API requires a replication policy. An empty automatic{}
	// marker can be pruned before it reaches the plugin, so re-assert it.
	if ctx.Operation != resource.OperationUpdate {
		repl, _ := body["replication"].(map[string]interface{})
		if repl == nil {
			repl = map[string]interface{}{}
		}
		_, hasAuto := repl["automatic"]
		_, hasUM := repl["userManaged"]
		if !hasAuto && !hasUM {
			repl["automatic"] = map[string]interface{}{}
		}
		body["replication"] = repl
	}

	return body, nil
}

// Create overrides BaseResource.Create for the POST+?secretId convention.
func (p *SecretProvisioner) Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error) {
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	var props map[string]interface{}
	if err := json.Unmarshal(req.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest, fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	name := utils.GetString(props, "name")
	if name == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest, "name is required"), nil
	}

	cfg := config.FromTargetConfig(req.TargetConfig, p.Config.Deps())
	pathCtx := base.PathContext{
		Project:      cfg.Project,
		ResourceType: p.ResourceConfig.ResourceType,
		ResourceName: name,
	}

	body := map[string]interface{}(props)
	if p.RequestTransformer != nil {
		body, err = p.RequestTransformer.Transform(props, base.TransformContext{
			Project:      pathCtx.Project,
			ResourceType: pathCtx.ResourceType,
			Operation:    resource.OperationCreate,
		})
		if err != nil {
			return createFailure(resource.OperationErrorCodeInvalidRequest, fmt.Sprintf("failed to transform request: %v", err)), nil
		}
	}

	urlBuilder := base.NewURLBuilder(p.APIConfig, pathCtx)
	url, err := transport.AddQueryParams(urlBuilder.CollectionURL(), map[string]string{"secretId": name})
	if err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest, fmt.Sprintf("failed to build URL: %v", err)), nil
	}

	response, err := client.SendRequest(ctx, transport.RequestOptions{Method: "POST", URL: url, Body: body})
	if err != nil {
		transportErr := transport.WrapError(err, fmt.Sprintf("failed to create %s '%s'", req.ResourceType, name))
		return createFailure(transport.ToResourceErrorCode(transportErr.Code), transportErr.Message), nil
	}

	nativeID := p.OperationConfig.NativeIDExtractor(response.Body, pathCtx)
	respBody := response.Body
	if p.ResponseTransformer != nil {
		respBody = p.ResponseTransformer.Transform(respBody, base.TransformContext{
			Project:      pathCtx.Project,
			ResourceType: pathCtx.ResourceType,
			Operation:    resource.OperationCreate,
		})
	}
	propsJSON, _ := json.Marshal(respBody)

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			NativeID:           nativeID,
			OperationStatus:    resource.OperationStatusSuccess,
			StatusMessage:      "Resource created successfully",
			ResourceProperties: propsJSON,
		},
	}, nil
}

func createFailure(code resource.OperationErrorCode, msg string) *resource.CreateResult {
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       code,
			StatusMessage:   msg,
		},
	}
}
