// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package gcp

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
)

type Client struct {
	config *config.Config
	ctx    context.Context
}

func NewClient(ctx context.Context, cfg *config.Config) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	return &Client{
		config: cfg,
		ctx:    ctx,
	}, nil
}

// CreateResource is a placeholder for generic resource creation
func (c *Client) CreateResource(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       resource.OperationErrorCodeNotUpdatable,
			StatusMessage:   fmt.Sprintf("Generic create not implemented for %s. Use custom provisioner.", request.ResourceType),
		},
	}, nil
}

// UpdateResource is a placeholder for generic resource update
func (c *Client) UpdateResource(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       resource.OperationErrorCodeNotUpdatable,
			StatusMessage:   fmt.Sprintf("Generic update not implemented for %s. Use custom provisioner.", request.ResourceType),
		},
	}, nil
}

// DeleteResource is a placeholder for generic resource deletion
func (c *Client) DeleteResource(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       resource.OperationErrorCodeNotUpdatable,
			StatusMessage:   fmt.Sprintf("Generic delete not implemented for %s. Use custom provisioner.", request.ResourceType),
		},
	}, nil
}

// ReadResource is a placeholder for generic resource reading
func (c *Client) ReadResource(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	return &resource.ReadResult{
		ResourceType: request.ResourceType,
		ErrorCode:    resource.OperationErrorCodeNotUpdatable,
	}, nil
}

// StatusResource checks the status of an async operation
func (c *Client) StatusResource(ctx context.Context, request *resource.StatusRequest, readFn func(context.Context, *resource.ReadRequest) (*resource.ReadResult, error)) (*resource.StatusResult, error) {
	// GCP operations complete synchronously in most cases
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
		},
	}, nil
}

// ListResources is a placeholder for generic resource listing
func (c *Client) ListResources(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	return &resource.ListResult{
		NativeIDs:     []string{},
		NextPageToken: nil,
	}, nil
}
