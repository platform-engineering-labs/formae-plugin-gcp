// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package gkehub

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// GKEHubProvisioner wraps BaseResource to handle the membershipId query parameter on create
type GKEHubProvisioner struct {
	*base.BaseResource
	resourceTypeAPI string // "memberships" or "features"
}

// Create overrides the base Create to add the membershipId query parameter
func (p *GKEHubProvisioner) Create(
	ctx context.Context,
	request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return failureResult(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	resourceName := utils.GetString(props, "name")
	if resourceName == "" {
		return failureResult(resource.OperationErrorCodeInvalidRequest,
			"resource name is required"), nil
	}

	cfg := config.FromTargetConfig(request.TargetConfig)
	pathCtx := base.PathContext{
		Project:      cfg.Project,
		Location:     cfg.Location,
		ResourceType: p.ResourceConfig.ResourceType,
		ResourceName: resourceName,
	}

	if pathCtx.Location == "" {
		pathCtx.Location = "global"
	}

	body := props
	if p.RequestTransformer != nil {
		transformCtx := base.TransformContext{
			Project:      pathCtx.Project,
			Location:     pathCtx.Location,
			ResourceType: pathCtx.ResourceType,
			Operation:    resource.OperationCreate,
		}
		body, err = p.RequestTransformer.Transform(props, transformCtx)
		if err != nil {
			return failureResult(resource.OperationErrorCodeInvalidRequest,
				fmt.Sprintf("failed to transform request: %v", err)), nil
		}
	}

	urlBuilder := base.NewURLBuilder(p.APIConfig, pathCtx)
	url := urlBuilder.CollectionURL()

	idParam := "membershipId"
	if p.resourceTypeAPI == "features" {
		idParam = "featureId"
	}
	url, err = transport.AddQueryParams(url, map[string]string{
		idParam: resourceName,
	})
	if err != nil {
		return failureResult(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to build URL: %v", err)), nil
	}

	response, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "POST",
		URL:    url,
		Body:   body,
	})
	if err != nil {
		transportErr := transport.WrapError(err, "failed to create membership")
		return failureResult(transport.ToResourceErrorCode(transportErr.Code),
			transportErr.Message), nil
	}

	nativeID := p.OperationConfig.NativeIDExtractor(response.Body, pathCtx)
	operationID := p.OperationConfig.OperationIDExtractor(response.Body)
	requestID := p.OperationConfig.OperationURLBuilder(pathCtx, operationID)

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        nativeID,
			RequestID:       requestID,
			StatusMessage:   fmt.Sprintf("%s creation in progress", p.ResourceConfig.ResourceType),
		},
	}, nil
}

// Status overrides the base Status to fetch resource properties when operation completes
func (p *GKEHubProvisioner) Status(
	ctx context.Context,
	request *resource.StatusRequest,
) (*resource.StatusResult, error) {
	result, err := p.BaseResource.Status(ctx, request)
	if err != nil {
		return result, err
	}

	if result.ProgressResult == nil || result.ProgressResult.OperationStatus != resource.OperationStatusSuccess {
		return result, nil
	}

	nativeID := result.ProgressResult.NativeID
	if nativeID == "" {
		return result, nil
	}

	readResult, err := p.Read(ctx, &resource.ReadRequest{
		ResourceType: request.ResourceType,
		NativeID:     nativeID,
		TargetConfig: request.TargetConfig,
	})
	if err != nil {
		return result, nil
	}

	if readResult.Properties != "" && readResult.ErrorCode == "" {
		result.ProgressResult.ResourceProperties = json.RawMessage(readResult.Properties)
	}

	return result, nil
}

func failureResult(errorCode resource.OperationErrorCode, message string) *resource.CreateResult {
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       errorCode,
			StatusMessage:   message,
		},
	}
}

func newGKEHubProvisioner(baseResource *base.BaseResource, resourceTypeAPI string) *GKEHubProvisioner {
	return &GKEHubProvisioner{
		BaseResource:    baseResource,
		resourceTypeAPI: resourceTypeAPI,
	}
}
