// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigtable

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// BigtableProvisioner wraps BaseResource to handle Bigtable-specific Create behavior
type BigtableProvisioner struct {
	*base.BaseResource
	resourceTypeAPI string // "instances", "clusters", "tables"
}

// Create overrides the base Create to add Bigtable-specific query parameters
func (p *BigtableProvisioner) Create(
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
		return createBigtableFailureResult(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	// Extract resource name for query parameter
	resourceName := utils.GetString(props, "name")
	if resourceName == "" {
		return createBigtableFailureResult(resource.OperationErrorCodeInvalidRequest,
			"resource name is required"), nil
	}

	// Build path context from config and properties
	cfg := config.FromTargetConfig(request.TargetConfig)
	pathCtx := base.PathContext{
		Project:      cfg.Project,
		ResourceType: p.ResourceConfig.ResourceType,
		ResourceName: resourceName,
	}

	// For nested resources, set parent
	if p.ResourceConfig.ParentResource != nil {
		parentProp := p.ResourceConfig.ParentResource.PropertyName
		if parentProp == "" {
			parentProp = p.ResourceConfig.ParentResource.ParentType
		}
		parent := utils.GetString(props, parentProp)
		if parent == "" && p.ResourceConfig.ParentResource.RequiresParent {
			return createBigtableFailureResult(resource.OperationErrorCodeInvalidRequest,
				fmt.Sprintf("%s is required for nested resources", parentProp)), nil
		}
		pathCtx.ParentResource = parent
		pathCtx.ParentType = p.ResourceConfig.ParentResource.ParentType

		// For backups (three-level hierarchy), also extract cluster name
		if p.ResourceConfig.ResourceType == "backups" {
			cluster := utils.GetString(props, "cluster")
			if cluster == "" {
				return createBigtableFailureResult(resource.OperationErrorCodeInvalidRequest,
					"cluster is required for backup resources"), nil
			}
			pathCtx.CustomSegments = []string{cluster}
		}
	}

	// Transform request properties
	body := props
	if p.RequestTransformer != nil {
		transformCtx := base.TransformContext{
			Project:      pathCtx.Project,
			ResourceType: pathCtx.ResourceType,
			Operation:    resource.OperationCreate,
		}
		body, err = p.RequestTransformer.Transform(props, transformCtx)
		if err != nil {
			return createBigtableFailureResult(resource.OperationErrorCodeInvalidRequest,
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

	// Add Bigtable-specific query parameter for resource ID
	// Format: ?instanceId={name} or ?clusterId={name} or ?tableId={name}
	resourceIDSuffix := strings.TrimSuffix(p.resourceTypeAPI, "s")
	resourceIDParam := resourceIDSuffix + "Id"
	url, err = transport.AddQueryParams(url, map[string]string{
		resourceIDParam: resourceName,
	})
	if err != nil {
		return createBigtableFailureResult(resource.OperationErrorCodeInvalidRequest,
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
		errMsg := fmt.Sprintf("Create failed for %s: %s (URL: %s)", request.ResourceType, transportErr.Message, url)
		return createBigtableFailureResult(transport.ToResourceErrorCode(transportErr.Code),
			errMsg), nil
	}

	// Check if this is a long-running operation
	if operationName, ok := response.Body["name"].(string); ok {
		// If it contains "operations", it's a long-running operation
		if strings.Contains(operationName, "/operations/") {
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

		// Otherwise, it's a synchronous response with the created resource
		nativeID := p.OperationConfig.NativeIDExtractor(response.Body, pathCtx)

		// Transform response if transformer provided
		responseBody := response.Body
		if p.ResponseTransformer != nil {
			transformCtx := base.TransformContext{
				Project:      pathCtx.Project,
				ResourceType: pathCtx.ResourceType,
				Operation:    resource.OperationCreate,
			}
			responseBody = p.ResponseTransformer.Transform(response.Body, transformCtx)
		}

		propsJSON, _ := json.Marshal(responseBody)

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

	return createBigtableFailureResult(resource.OperationErrorCodeUnforeseenError,
		"Invalid response: missing name field"), nil
}

// createBigtableFailureResult creates a failure result
func createBigtableFailureResult(errorCode resource.OperationErrorCode, message string) *resource.CreateResult {
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       errorCode,
			StatusMessage:   message,
		},
	}
}

// newBigtableProvisionerWithBase creates a BigtableProvisioner from a BaseResource
func newBigtableProvisionerWithBase(baseResource *base.BaseResource, resourceTypeAPI string) *BigtableProvisioner {
	return &BigtableProvisioner{
		BaseResource:    baseResource,
		resourceTypeAPI: resourceTypeAPI,
	}
}
