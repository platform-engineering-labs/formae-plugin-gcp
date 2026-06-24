// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package pubsub

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

// Resource type constants
const (
	TopicResourceType        = "GCP::PubSub::Topic"
	SubscriptionResourceType = "GCP::PubSub::Subscription"
	SchemaResourceType       = "GCP::PubSub::Schema"
)

// createStyle distinguishes Pub/Sub's two create conventions.
type createStyle int

const (
	// createPut: PUT to the resource URL (topics, subscriptions).
	createPut createStyle = iota
	// createPostQueryID: POST to the collection URL with a ?<idParam>=<name>
	// query parameter (schemas).
	createPostQueryID
)

var pubsubRegistry *base.ResourceRegistry

// PubSubProvisioner wraps BaseResource to handle Pub/Sub-specific Create.
// Read/Update/Delete/List/Status are inherited from BaseResource (they operate
// on the full resource path, which Pub/Sub uses natively).
type PubSubProvisioner struct {
	*base.BaseResource
	style   createStyle
	idParam string // query-param name for createPostQueryID style
}

func init() {
	pubsubRegistry = base.NewResourceRegistry(PubSubAPI, PubSubOperations, PubSubNativeID)

	err := pubsubRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: TopicResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "topics",
				SupportsUpdate: false, // ponytail: labels/retention are updatable via PATCH+updateMask; defer until verified
			},
			RequestTransformer: base.RequestTransformerFunc(stripName),
		},
		{
			ResourceType: SubscriptionResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "subscriptions",
				SupportsUpdate: false, // ponytail: PATCH+updateMask; defer until verified
			},
			RequestTransformer: base.RequestTransformerFunc(stripName),
		},
		{
			ResourceType: SchemaResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "schemas",
				SupportsUpdate: false, // schemas are immutable
			},
			RequestTransformer: base.RequestTransformerFunc(stripName),
		},
	})
	if err != nil {
		panic(err)
	}

	// Override with PubSubProvisioner to handle the non-standard create paths.
	styles := map[string]struct {
		style   createStyle
		idParam string
	}{
		TopicResourceType:        {createPut, ""},
		SubscriptionResourceType: {createPut, ""},
		SchemaResourceType:       {createPostQueryID, "schemaId"},
	}
	for rt, s := range styles {
		resourceType, st := rt, s
		registry.Register(
			resourceType,
			[]resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			func(cfg *config.Config) prov.Provisioner {
				def := pubsubRegistry.Definitions[resourceType]
				baseResource := &base.BaseResource{
					Config:             cfg,
					APIConfig:          PubSubAPI,
					OperationConfig:    PubSubOperations,
					ResourceConfig:     def.ResourceConfig,
					NativeIDConfig:     PubSubNativeID,
					RequestTransformer: def.RequestTransformer,
				}
				return &PubSubProvisioner{BaseResource: baseResource, style: st.style, idParam: st.idParam}
			},
		)
	}
}

// stripName removes the "name" property from the request body. The short name
// is carried in the URL path (PUT) or the ?<id> query param (POST); Pub/Sub
// rejects a short name in the body.
func stripName(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		if k == "name" {
			continue
		}
		body[k] = v
	}
	return body, nil
}

// Create overrides BaseResource.Create for Pub/Sub's PUT / POST?id conventions.
func (p *PubSubProvisioner) Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error) {
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

	cfg := config.FromTargetConfig(req.TargetConfig)
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
	var method, url string
	switch p.style {
	case createPut:
		method = "PUT"
		url = urlBuilder.ResourceURL(name)
	case createPostQueryID:
		method = "POST"
		url, err = transport.AddQueryParams(urlBuilder.CollectionURL(), map[string]string{p.idParam: name})
		if err != nil {
			return createFailure(resource.OperationErrorCodeInvalidRequest, fmt.Sprintf("failed to build URL: %v", err)), nil
		}
	}

	response, err := client.SendRequest(ctx, transport.RequestOptions{Method: method, URL: url, Body: body})
	if err != nil {
		transportErr := transport.WrapError(err, fmt.Sprintf("failed to create %s '%s'", req.ResourceType, name))
		return createFailure(transport.ToResourceErrorCode(transportErr.Code), transportErr.Message), nil
	}

	nativeID := p.OperationConfig.NativeIDExtractor(response.Body, pathCtx)
	propsJSON, _ := json.Marshal(response.Body)

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
