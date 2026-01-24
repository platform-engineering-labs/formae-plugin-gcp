// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package cloudrun

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
)

// CloudRunProvisioner wraps BaseResource to handle Cloud Run-specific Create behavior
type CloudRunProvisioner struct {
	*base.BaseResource
	resourceTypeAPI string // "services" or "jobs"
}

// Create overrides the base Create to add Cloud Run-specific query parameters
func (p *CloudRunProvisioner) Create(
	ctx context.Context,
	request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	// Parse properties
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailureResult(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	// Extract resource name for query parameter
	resourceName := utils.GetString(props, "name")
	if resourceName == "" {
		return createFailureResult(resource.OperationErrorCodeInvalidRequest,
			"resource name is required"), nil
	}

	// Build path context from config and properties
	cfg := config.FromTargetConfig(request.TargetConfig)
	pathCtx := base.PathContext{
		Project:      cfg.Project,
		Region:       cfg.Region,
		Zone:         cfg.Zone,
		Location:     cfg.Region, // Cloud Run uses location
		ResourceType: p.ResourceConfig.ResourceType,
		ResourceName: resourceName,
	}

	// Override location from properties if specified
	if location := utils.GetString(props, "location"); location != "" {
		pathCtx.Location = location
		pathCtx.Region = location
	}

	// Transform request properties
	body := props
	if p.RequestTransformer != nil {
		transformCtx := base.TransformContext{
			Project:      pathCtx.Project,
			Region:       pathCtx.Region,
			Zone:         pathCtx.Zone,
			Location:     pathCtx.Location,
			ResourceType: pathCtx.ResourceType,
			Operation:    resource.OperationCreate,
		}
		body, err = p.RequestTransformer.Transform(props, transformCtx)
		if err != nil {
			return createFailureResult(resource.OperationErrorCodeInvalidRequest,
				fmt.Sprintf("failed to transform request: %v", err)), nil
		}
	}

	// Apply request wrapper if configured
	if p.ResourceConfig.RequestWrapper != "" {
		body = map[string]interface{}{
			p.ResourceConfig.RequestWrapper: body,
		}
	}

	// Build URL (without resource name in path for collection URL)
	urlBuilder := base.NewURLBuilder(p.APIConfig, pathCtx)
	url := urlBuilder.CollectionURL()

	// Add Cloud Run-specific query parameter for resource ID
	idParam := "serviceId"
	if p.resourceTypeAPI == "jobs" {
		idParam = "jobId"
	}
	url, err = transport.AddQueryParams(url, map[string]string{
		idParam: resourceName,
	})
	if err != nil {
		return createFailureResult(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to build URL: %v", err)), nil
	}

	// Send create request
	response, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "POST",
		URL:    url,
		Body:   body,
	})
	if err != nil {
		transportErr := transport.WrapError(err, "failed to create resource")
		return createFailureResult(transport.ToResourceErrorCode(transportErr.Code),
			transportErr.Message), nil
	}

	// Extract native ID and operation ID for async operations
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

// createFailureResult creates a failure result
func createFailureResult(errorCode resource.OperationErrorCode, message string) *resource.CreateResult {
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       errorCode,
			StatusMessage:   message,
		},
	}
}

// newCloudRunProvisionerWithBase creates a CloudRunProvisioner from a BaseResource
func newCloudRunProvisionerWithBase(baseResource *base.BaseResource, resourceTypeAPI string) *CloudRunProvisioner {
	return &CloudRunProvisioner{
		BaseResource:    baseResource,
		resourceTypeAPI: resourceTypeAPI,
	}
}
