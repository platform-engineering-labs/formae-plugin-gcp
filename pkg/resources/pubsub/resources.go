// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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
	SnapshotResourceType     = "GCP::PubSub::Snapshot"
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

	// Update support. Pub/Sub PATCH wraps the resource under updateWrapper
	// (e.g. "topic") alongside an "updateMask" body field. Empty updateWrapper
	// means the resource is immutable (no Update).
	updateWrapper string
	mutableFields map[string]bool
}

func init() {
	pubsubRegistry = base.NewResourceRegistry(PubSubAPI, PubSubOperations, PubSubNativeID)

	err := pubsubRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType:        TopicResourceType,
			ResourceConfig:      base.ResourceConfig{ResourceType: "topics"},
			RequestTransformer:  base.RequestTransformerFunc(stripName),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			ResourceType:        SubscriptionResourceType,
			ResourceConfig:      base.ResourceConfig{ResourceType: "subscriptions"},
			RequestTransformer:  base.RequestTransformerFunc(subscriptionCreateTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(subscriptionResponseTransformer),
		},
		{
			ResourceType:        SnapshotResourceType,
			ResourceConfig:      base.ResourceConfig{ResourceType: "snapshots"},
			RequestTransformer:  base.RequestTransformerFunc(snapshotCreateTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(snapshotResponseTransformer),
		},
		{
			ResourceType:        SchemaResourceType,
			ResourceConfig:      base.ResourceConfig{ResourceType: "schemas"}, // immutable
			RequestTransformer:  base.RequestTransformerFunc(stripName),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
	})
	if err != nil {
		panic(err)
	}

	// Per-resource create/update conventions. Pub/Sub create uses PUT to the
	// named resource (topics/subscriptions) or POST+?schemaId (schemas); update
	// is a PATCH wrapping the resource under a singular key + updateMask.
	type rconf struct {
		style         createStyle
		idParam       string
		updateWrapper string          // "" => immutable
		mutableFields map[string]bool // fields a PATCH may set
	}
	configs := map[string]rconf{
		TopicResourceType: {createPut, "", "topic", map[string]bool{
			"labels": true, "messageRetentionDuration": true,
		}},
		SubscriptionResourceType: {createPut, "", "subscription", map[string]bool{
			"ackDeadlineSeconds": true, "retainAckedMessages": true,
			"messageRetentionDuration": true, "labels": true,
		}},
		SnapshotResourceType: {createPut, "", "snapshot", map[string]bool{
			"labels": true,
		}},
		SchemaResourceType: {createPostQueryID, "schemaId", "", nil},
	}

	for rt, c := range configs {
		resourceType, rc := rt, c
		ops := []resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		}
		if rc.updateWrapper != "" {
			ops = append(ops, resource.OperationUpdate)
		}
		registry.Register(
			resourceType,
			ops,
			func(cfg *config.Config) prov.Provisioner {
				def := pubsubRegistry.Definitions[resourceType]
				baseResource := &base.BaseResource{
					Config:              cfg,
					APIConfig:           PubSubAPI,
					OperationConfig:     PubSubOperations,
					ResourceConfig:      def.ResourceConfig,
					NativeIDConfig:      PubSubNativeID,
					RequestTransformer:  def.RequestTransformer,
					ResponseTransformer: def.ResponseTransformer,
				}
				return &PubSubProvisioner{
					BaseResource:  baseResource,
					style:         rc.style,
					idParam:       rc.idParam,
					updateWrapper: rc.updateWrapper,
					mutableFields: rc.mutableFields,
				}
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

// subscriptionCreateTransformer drops "name" and expands "topic" to the full
// resource path the API requires (the schema/refs carry the short topic name).
func subscriptionCreateTransformer(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	body, _ := stripName(props, ctx)
	if topic, ok := body["topic"].(string); ok && topic != "" && !strings.HasPrefix(topic, "projects/") {
		body["topic"] = fmt.Sprintf("projects/%s/topics/%s", ctx.Project, topic)
	}
	return body, nil
}

// subscriptionResponseTransformer normalizes the full-path "name" and "topic"
// fields back to their short forms so stored state matches declared state.
func subscriptionResponseTransformer(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
	shortenField(apiResponse, "name")
	shortenField(apiResponse, "topic")
	return apiResponse
}

// snapshotCreateTransformer builds CreateSnapshotRequest. The create body is
// not the Snapshot resource: it carries the "subscription" to snapshot (which
// the API never echoes back — it reports that subscription's "topic" instead),
// so every read-only field is dropped and "subscription" is expanded to the
// full resource path the API requires.
func snapshotCreateTransformer(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		switch k {
		case "name", "topic", "expireTime":
			continue
		}
		body[k] = v
	}
	if sub, ok := body["subscription"].(string); ok && sub != "" && !strings.HasPrefix(sub, "projects/") {
		body["subscription"] = fmt.Sprintf("projects/%s/subscriptions/%s", ctx.Project, sub)
	}
	return body, nil
}

func snapshotResponseTransformer(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
	shortenField(apiResponse, "name")
	shortenField(apiResponse, "topic")
	return apiResponse
}

func shortenField(m map[string]interface{}, key string) {
	if v, ok := m[key].(string); ok {
		if i := strings.LastIndex(v, "/"); i >= 0 {
			m[key] = v[i+1:]
		}
	}
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

	// Native ID is the full resource path - compute it before the response
	// transformer shortens "name".
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

// Update overrides BaseResource.Update for Pub/Sub's PATCH convention: the
// resource is wrapped under a singular key ("topic"/"subscription") alongside
// an "updateMask" body field listing only the mutable fields being set.
func (p *PubSubProvisioner) Update(ctx context.Context, req *resource.UpdateRequest) (*resource.UpdateResult, error) {
	if p.updateWrapper == "" {
		return updateFailure(req.NativeID, resource.OperationErrorCodeNotUpdatable,
			fmt.Sprintf("%s does not support updates", p.ResourceConfig.ResourceType)), nil
	}

	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	var props map[string]interface{}
	if err := json.Unmarshal(req.DesiredProperties, &props); err != nil {
		return updateFailure(req.NativeID, resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	pathCtx, err := base.ParseNativeID(p.NativeIDConfig, req.NativeID)
	if err != nil {
		return updateFailure(req.NativeID, resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid native ID: %v", err)), nil
	}
	pathCtx.ResourceType = p.ResourceConfig.ResourceType

	// Keep only mutable fields; build the updateMask from exactly those.
	resourceBody := make(map[string]interface{})
	fields := make([]string, 0, len(props))
	for k, v := range props {
		if p.mutableFields[k] {
			resourceBody[k] = v
			fields = append(fields, k)
		}
	}
	sort.Strings(fields)

	body := map[string]interface{}{
		p.updateWrapper: resourceBody,
		"updateMask":    strings.Join(fields, ","),
	}

	url := base.NewURLBuilder(p.APIConfig, pathCtx).ResourceURL(pathCtx.ResourceName)
	_, err = client.SendRequest(ctx, transport.RequestOptions{Method: "PATCH", URL: url, Body: body})
	if err != nil {
		transportErr := transport.WrapError(err, fmt.Sprintf("failed to update %s", req.ResourceType))
		return updateFailure(req.NativeID, transport.ToResourceErrorCode(transportErr.Code), transportErr.Message), nil
	}

	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			NativeID:        req.NativeID,
			OperationStatus: resource.OperationStatusSuccess,
			StatusMessage:   "Resource updated successfully",
		},
	}, nil
}

func updateFailure(nativeID string, code resource.OperationErrorCode, msg string) *resource.UpdateResult {
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       code,
			StatusMessage:   msg,
			NativeID:        nativeID,
		},
	}
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
