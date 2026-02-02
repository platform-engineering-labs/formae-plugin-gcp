// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// DiskProvisioner wraps BaseResource to handle Disk-specific Update behavior
// Disks use the setLabels endpoint for label updates instead of standard PATCH/PUT
type DiskProvisioner struct {
	*base.BaseResource
}

// Ensure DiskProvisioner implements prov.Provisioner
var _ prov.Provisioner = &DiskProvisioner{}

// Update overrides the base Update to use the setLabels endpoint
// GCP Compute Disk API: POST /projects/{project}/zones/{zone}/disks/{disk}/setLabels
// Request body: {"labels": {...}, "labelFingerprint": "..."}
func (p *DiskProvisioner) Update(
	ctx context.Context,
	request *resource.UpdateRequest,
) (*resource.UpdateResult, error) {
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	// Parse properties
	var props map[string]interface{}
	if err := json.Unmarshal(request.DesiredProperties, &props); err != nil {
		return p.updateFailureResult(request.NativeID,
			resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}

	// Parse native ID
	pathCtx, err := base.ParseNativeID(p.NativeIDConfig, request.NativeID)
	if err != nil {
		return p.updateFailureResult(request.NativeID,
			resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid native ID: %v", err)), nil
	}

	p.FillPathContextFromTarget(request.TargetConfig, &pathCtx)
	pathCtx.ResourceType = p.ResourceConfig.ResourceType

	// First, read the current disk to get the labelFingerprint
	urlBuilder := base.NewURLBuilder(p.APIConfig, pathCtx)
	resourceURL := urlBuilder.ResourceURL(pathCtx.ResourceName)

	getResponse, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    resourceURL,
	})
	if err != nil {
		wrappedErr := transport.WrapError(err, "failed to read disk for label update")
		return p.updateFailureResult(request.NativeID,
			transport.ToResourceErrorCode(wrappedErr.Code),
			wrappedErr.Message), nil
	}

	// Extract labelFingerprint from current resource
	labelFingerprint, _ := getResponse.Body["labelFingerprint"].(string)

	// Build setLabels request body
	labels := utils.GetObject(props, "labels")
	if labels == nil {
		labels = make(map[string]interface{})
	}

	setLabelsBody := map[string]interface{}{
		"labels":           labels,
		"labelFingerprint": labelFingerprint,
	}

	// Build setLabels URL
	setLabelsURL := resourceURL + "/setLabels"

	// Send POST request to setLabels
	response, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "POST",
		URL:    setLabelsURL,
		Body:   setLabelsBody,
	})
	if err != nil {
		transportErr := transport.WrapError(err, "failed to update disk labels")
		return p.updateFailureResult(request.NativeID,
			transport.ToResourceErrorCode(transportErr.Code),
			transportErr.Message), nil
	}

	// Extract operation ID for async operation
	operationID := p.OperationConfig.OperationIDExtractor(response.Body)
	requestID := p.OperationConfig.OperationURLBuilder(pathCtx, operationID)

	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        request.NativeID,
			RequestID:       requestID,
			StatusMessage:   fmt.Sprintf("%s label update in progress", p.ResourceConfig.ResourceType),
		},
	}, nil
}

// updateFailureResult creates a failure result for update operations
func (p *DiskProvisioner) updateFailureResult(nativeID string, errorCode resource.OperationErrorCode, message string) *resource.UpdateResult {
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       errorCode,
			StatusMessage:   message,
			NativeID:        nativeID,
		},
	}
}

// NewDiskProvisioner creates a DiskProvisioner from a BaseResource
func NewDiskProvisioner(baseResource *base.BaseResource) *DiskProvisioner {
	return &DiskProvisioner{
		BaseResource: baseResource,
	}
}

// Create delegates to BaseResource
func (p *DiskProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	return p.BaseResource.Create(ctx, request)
}

// Read delegates to BaseResource
func (p *DiskProvisioner) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	return p.BaseResource.Read(ctx, request)
}

// Delete delegates to BaseResource
func (p *DiskProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	return p.BaseResource.Delete(ctx, request)
}

// List delegates to BaseResource
func (p *DiskProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	return p.BaseResource.List(ctx, request)
}

// Status delegates to BaseResource and reads resource on success
func (p *DiskProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	result, err := p.BaseResource.Status(ctx, request)
	if err != nil {
		return nil, err
	}

	// For successful operations, read the resource to get properties
	if result.ProgressResult.OperationStatus == resource.OperationStatusSuccess &&
		result.ProgressResult.NativeID != "" {
		readResult, err := p.Read(ctx, &resource.ReadRequest{
			NativeID:     result.ProgressResult.NativeID,
			ResourceType: request.ResourceType,
			TargetConfig: request.TargetConfig,
		})
		if err == nil && readResult.ErrorCode == "" {
			result.ProgressResult.ResourceProperties = []byte(readResult.Properties)
		}
	}

	return result, nil
}
