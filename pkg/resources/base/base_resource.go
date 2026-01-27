// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package base

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
)

// BaseResource provides unified CRUD operations for all GCP APIs
type BaseResource struct {
	Config             *config.Config
	APIConfig          APIConfig
	OperationConfig    OperationConfig
	ResourceConfig     ResourceConfig
	NativeIDConfig     NativeIDConfig
	RequestTransformer RequestTransformer
	ResponseTransformer ResponseTransformer
}

// Create performs a CREATE operation for a resource
func (b *BaseResource) Create(
	ctx context.Context,
	request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	client, err := transport.NewClient(ctx, b.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	// Parse properties
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return b.createFailureResult(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	// Build path context
	pathCtx := b.buildPathContext(request.TargetConfig, props)

	// Transform request properties
	body := props
	if b.RequestTransformer != nil {
		transformCtx := b.buildTransformContext(pathCtx, resource.OperationCreate)
		body, err = b.RequestTransformer.Transform(props, transformCtx)
		if err != nil {
			return b.createFailureResult(resource.OperationErrorCodeInvalidRequest,
				fmt.Sprintf("failed to transform request: %v", err)), nil
		}
	}

	// Apply request wrapper if configured
	if b.ResourceConfig.RequestWrapper != "" {
		body = map[string]interface{}{
			b.ResourceConfig.RequestWrapper: body,
		}
	}

	// Build URL
	urlBuilder := NewURLBuilder(b.APIConfig, pathCtx)
	url := urlBuilder.CollectionURL()

	// Send create request
	response, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "POST",
		URL:    url,
		Body:   body,
	})
	if err != nil {
		// Include resource type and name in error context for better debugging
		resourceName := pathCtx.ResourceName
		if resourceName == "" {
			if name, ok := props["name"].(string); ok {
				resourceName = name
			}
		}
		contextMsg := fmt.Sprintf("failed to create %s", request.ResourceType)
		if resourceName != "" {
			contextMsg = fmt.Sprintf("failed to create %s '%s'", request.ResourceType, resourceName)
		}
		transportErr := transport.WrapError(err, contextMsg)
		return b.createFailureResult(transport.ToResourceErrorCode(transportErr.Code),
			transportErr.Message), nil
	}

	// Handle synchronous operations
	if b.OperationConfig.Synchronous {
		return b.handleSynchronousCreate(ctx, request, response.Body, pathCtx)
	}

	// Extract native ID and operation ID for async operations
	nativeID := b.OperationConfig.NativeIDExtractor(response.Body, pathCtx)
	operationID := b.OperationConfig.OperationIDExtractor(response.Body)
	requestID := b.OperationConfig.OperationURLBuilder(pathCtx, operationID)

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        nativeID,
			RequestID:       requestID,
			StatusMessage:   fmt.Sprintf("%s creation in progress", b.ResourceConfig.ResourceType),
		},
	}, nil
}

// Read performs a READ operation for a resource
func (b *BaseResource) Read(
	ctx context.Context,
	request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	client, err := transport.NewClient(ctx, b.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	// Parse native ID to get path context
	pathCtx, err := ParseNativeID(b.NativeIDConfig, request.NativeID)
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeInvalidRequest,
		}, nil
	}

	// Fill in missing context from target
	b.fillPathContextFromTarget(request.TargetConfig, &pathCtx)
	pathCtx.ResourceType = b.ResourceConfig.ResourceType

	// Build URL using just the resource name (not the full native ID)
	urlBuilder := NewURLBuilder(b.APIConfig, pathCtx)
	url := urlBuilder.ResourceURL(pathCtx.ResourceName)

	// Send GET request
	response, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    url,
	})
	if err != nil {
		wrappedErr := transport.WrapError(err, "failed to read resource")
		return &resource.ReadResult{
			ErrorCode: transport.ToResourceErrorCode(wrappedErr.Code),
		}, nil
	}

	// Transform response if configured
	apiResponse := response.Body
	if b.ResponseTransformer != nil {
		transformCtx := b.buildTransformContext(pathCtx, resource.OperationRead)
		apiResponse = b.ResponseTransformer.Transform(apiResponse, transformCtx)
	}

	// Marshal to JSON
	propsJSON, err := json.Marshal(apiResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal properties: %w", err)
	}

	return &resource.ReadResult{
		Properties: string(propsJSON),
	}, nil
}

// Update performs an UPDATE operation for a resource
func (b *BaseResource) Update(
	ctx context.Context,
	request *resource.UpdateRequest,
) (*resource.UpdateResult, error) {
	if !b.ResourceConfig.SupportsUpdate {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeNotUpdatable,
				StatusMessage:   fmt.Sprintf("%s does not support updates", b.ResourceConfig.ResourceType),
				NativeID:        request.NativeID,
			},
		}, nil
	}

	client, err := transport.NewClient(ctx, b.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	// Parse properties
	var props map[string]interface{}
	if err := json.Unmarshal(request.DesiredProperties, &props); err != nil {
		return b.updateFailureResult(request.NativeID,
			resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	// Parse native ID
	pathCtx, err := ParseNativeID(b.NativeIDConfig, request.NativeID)
	if err != nil {
		return b.updateFailureResult(request.NativeID,
			resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid native ID: %v", err)), nil
	}

	b.fillPathContextFromTarget(request.TargetConfig, &pathCtx)
	pathCtx.ResourceType = b.ResourceConfig.ResourceType

	// Handle optimistic locking
	if b.ResourceConfig.OptimisticLocking != nil && b.ResourceConfig.OptimisticLocking.Enabled {
		return b.updateWithOptimisticLocking(ctx, client, request, props, pathCtx)
	}

	// Standard update without locking
	return b.performUpdate(ctx, client, request, props, pathCtx)
}

// Delete performs a DELETE operation for a resource
func (b *BaseResource) Delete(
	ctx context.Context,
	request *resource.DeleteRequest,
) (*resource.DeleteResult, error) {
	client, err := transport.NewClient(ctx, b.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	nativeID := request.NativeID

	// Parse native ID
	pathCtx, err := ParseNativeID(b.NativeIDConfig, nativeID)
	if err != nil {
		return b.deleteFailureResult(nativeID,
			resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid native ID: %v", err)), nil
	}

	b.fillPathContextFromTarget(request.TargetConfig, &pathCtx)
	pathCtx.ResourceType = b.ResourceConfig.ResourceType

	// Build URL using just the resource name (not the full native ID)
	urlBuilder := NewURLBuilder(b.APIConfig, pathCtx)
	url := urlBuilder.ResourceURL(pathCtx.ResourceName)

	// Send DELETE request
	response, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "DELETE",
		URL:    url,
	})
	if err != nil {
		wrappedErr := transport.WrapError(err, "failed to delete resource")
		errorCode := transport.ToResourceErrorCode(wrappedErr.Code)

		// 404 is success for delete
		if errorCode == resource.OperationErrorCodeNotFound {
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusSuccess,
					NativeID:        nativeID,
					StatusMessage:   fmt.Sprintf("%s already deleted", b.ResourceConfig.ResourceType),
				},
			}, nil
		}

		return b.deleteFailureResult(nativeID, errorCode, wrappedErr.Message), nil
	}

	// Handle synchronous operations
	if b.OperationConfig.Synchronous {
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusSuccess,
				NativeID:        nativeID,
				StatusMessage:   "Resource deleted successfully",
			},
		}, nil
	}

	// Extract operation ID for async operations
	operationID := b.OperationConfig.OperationIDExtractor(response.Body)

	// If operation ID is empty, the operation completed synchronously
	// (common for delete operations that return Empty responses)
	if operationID == "" {
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusSuccess,
				NativeID:        nativeID,
				StatusMessage:   "Resource deleted successfully",
			},
		}, nil
	}

	requestID := b.OperationConfig.OperationURLBuilder(pathCtx, operationID)

	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        nativeID,
			RequestID:       requestID,
			StatusMessage:   fmt.Sprintf("%s deletion in progress", b.ResourceConfig.ResourceType),
		},
	}, nil
}

// List performs a LIST operation for resources
func (b *BaseResource) List(
	ctx context.Context,
	request *resource.ListRequest,
) (*resource.ListResult, error) {
	client, err := transport.NewClient(ctx, b.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	// Build path context from target
	cfg := config.FromTargetConfig(request.TargetConfig)
	// Use explicit Location only - no fallback to Region
	pathCtx := PathContext{
		Project:      cfg.Project,
		Region:       cfg.Region,
		Zone:         cfg.Zone,
		Location:     cfg.Location, // Container/CloudRun use location (no Region fallback)
		ResourceType: b.ResourceConfig.ResourceType,
	}

	// Respect resource scope
	if b.ResourceConfig.Scope != nil {
		switch b.ResourceConfig.Scope.Type {
		case ScopeGlobal:
			// Global resources don't use region or zone
			pathCtx.Region = ""
			pathCtx.Zone = ""
			pathCtx.Location = ""
		case ScopeRegional:
			// Regional resources don't use zone
			pathCtx.Zone = ""
		case ScopeZonal:
			// Zonal resources use both region and zone
			// Keep both as-is
		case ScopeLocationBased:
			// Location-based resources (Container/GKE, CloudRun) require explicit location
			// If location is not provided, return empty result instead of making API call
			if pathCtx.Location == "" {
				return &resource.ListResult{
					NativeIDs:     []string{},
					NextPageToken: nil,
				}, nil
			}
		}
	}

	// Handle parent resource for nested resources
	if b.ResourceConfig.ParentResource != nil && b.ResourceConfig.ParentResource.RequiresParent {
		if request.AdditionalProperties == nil {
			return nil, fmt.Errorf("nested resource %s requires parent info in AdditionalProperties", request.ResourceType)
		}
		// Extract parent info from additional properties
		// Use PropertyName if specified, otherwise fall back to ParentType
		propName := b.ResourceConfig.ParentResource.PropertyName
		if propName == "" {
			propName = b.ResourceConfig.ParentResource.ParentType
		}
		if parent, ok := request.AdditionalProperties[propName]; ok {
			pathCtx.ParentResource = parent
			pathCtx.ParentType = b.ResourceConfig.ParentResource.ParentType
		} else {
			return nil, fmt.Errorf("nested resource %s requires '%s' in AdditionalProperties", request.ResourceType, propName)
		}
	}

	// Build URL - for zonal/regional resources without a specific zone/region, use aggregated list
	var url string
	if b.ResourceConfig.Scope != nil && b.ResourceConfig.Scope.Type == ScopeZonal && pathCtx.Zone == "" {
		// Use aggregated list for zonal resources when no specific zone is provided
		// Format: /projects/{project}/aggregated/{resourceType}
		url = fmt.Sprintf("%s/projects/%s/aggregated/%s",
			b.APIConfig.BaseURL, pathCtx.Project, b.ResourceConfig.ResourceType)
	} else if b.ResourceConfig.Scope != nil && b.ResourceConfig.Scope.Type == ScopeRegional && pathCtx.Region == "" {
		// Use aggregated list for regional resources when no specific region is provided
		// Format: /projects/{project}/aggregated/{resourceType}
		url = fmt.Sprintf("%s/projects/%s/aggregated/%s",
			b.APIConfig.BaseURL, pathCtx.Project, b.ResourceConfig.ResourceType)
	} else {
		urlBuilder := NewURLBuilder(b.APIConfig, pathCtx)
		url = urlBuilder.CollectionURL()
	}

	// Add pagination parameters using API-specific parameter names (if not disabled)
	if !b.APIConfig.IsPaginationDisabled() {
		if request.PageSize > 0 {
			url, _ = transport.AddQueryParam(url, b.APIConfig.GetPageSizeParam(), fmt.Sprintf("%d", request.PageSize))
		}
		if request.PageToken != nil && *request.PageToken != "" {
			url, _ = transport.AddQueryParam(url, b.APIConfig.GetPageTokenParam(), *request.PageToken)
		}
	}

	// Send GET request
	response, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    url,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}

	// Parse items - this is API-specific
	// Most APIs return {"items": [...]} or {resourceType: [...]}
	return b.parseListResponse(response.Body, pathCtx)
}

// Status checks operation status
func (b *BaseResource) Status(
	ctx context.Context,
	request *resource.StatusRequest,
) (*resource.StatusResult, error) {
	// Synchronous APIs don't need status checking
	if b.OperationConfig.Synchronous {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusSuccess,
				StatusMessage:   "Operation completed synchronously",
				RequestID:       request.RequestID,
			},
		}, nil
	}

	client, err := transport.NewClient(ctx, b.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	// Build full URL for operation status check
	// RequestID contains the operation path (e.g., "projects/{project}/operations/{id}")
	operationURL := fmt.Sprintf("%s/%s", b.APIConfig.BaseURL, request.RequestID)

	// Get operation status
	response, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    operationURL,
	})
	if err != nil {
		wrappedErr := transport.WrapError(err, "failed to get operation status")
		statusMessage := fmt.Sprintf("%s (URL: %s)", wrappedErr.Message, operationURL)
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       transport.ToResourceErrorCode(wrappedErr.Code),
				StatusMessage:   statusMessage,
				RequestID:       request.RequestID,
			},
		}, nil
	}

	// Extract native ID from operation response (use empty PathContext since we don't need it for extraction)
	pathCtx := PathContext{}
	nativeID := b.OperationConfig.NativeIDExtractor(response.Body, pathCtx)

	// Check if operation is complete
	done, err := b.OperationConfig.OperationStatusChecker(response.Body)

	if err != nil {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeServiceInternalError,
				StatusMessage:   err.Error(),
				RequestID:       request.RequestID,
				NativeID:        nativeID,
			},
		}, nil
	}

	if !done {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusInProgress,
				StatusMessage:   "Operation in progress",
				RequestID:       request.RequestID,
				NativeID:        nativeID,
			},
		}, nil
	}

	// Operation completed successfully
	// Try to read the resource for create/update operations
	// This is handled by the caller (provisioner)
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
			StatusMessage:   "Operation completed successfully",
			RequestID:       request.RequestID,
			NativeID:        nativeID,
		},
	}, nil
}

// Helper methods continue in next part...
