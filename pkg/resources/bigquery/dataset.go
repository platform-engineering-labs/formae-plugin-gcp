// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigquery

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/status"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

type Dataset struct {
	cfg *config.Config
}

var _ prov.Provisioner = &Dataset{}

func init() {
	registry.Register("GCP::BigQuery::Dataset",
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
		},
		func(cfg *config.Config) prov.Provisioner {
			return &Dataset{cfg: cfg}
		})
}

func (d *Dataset) getClient(ctx context.Context, project string) (*bigquery.Client, error) {
	opts, err := d.cfg.ToClientOptions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create client options: %w", err)
	}

	client, err := bigquery.NewClient(ctx, project, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create BigQuery client: %w", err)
	}

	return client, nil
}

// Create creates a new BigQuery dataset
func (d *Dataset) Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error) {
	props, err := utils.ParseProperties(req.Properties)
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
				StatusMessage:   fmt.Sprintf("Failed to parse properties: %v", err),
			},
		}, nil
	}

	project := config.FromTargetConfig(req.TargetConfig).Project
	datasetID := utils.GetString(props, "datasetId")
	if datasetID == "" {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
				StatusMessage:   "datasetId is required",
			},
		}, nil
	}

	client, err := d.getClient(ctx, project)
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeUnforeseenError,
				StatusMessage:   fmt.Sprintf("Failed to create client: %v", err),
			},
		}, nil
	}
	defer func() { _ = client.Close() }()

	ds := client.Dataset(datasetID)
	metadata := &bigquery.DatasetMetadata{
		Name:        utils.GetString(props, "name"),
		Description: utils.GetString(props, "description"),
		Location:    utils.GetString(props, "location"),
		Labels:      getStringMap(props, "labels"),
	}

	// Default dataset expiration
	if expirationMs := utils.GetInt64(props, "defaultTableExpirationMs"); expirationMs > 0 {
		metadata.DefaultTableExpiration = time.Duration(expirationMs) * time.Millisecond
	}

	// Default partition expiration
	if partitionMs := utils.GetInt64(props, "defaultPartitionExpirationMs"); partitionMs > 0 {
		metadata.DefaultPartitionExpiration = time.Duration(partitionMs) * time.Millisecond
	}

	err = ds.Create(ctx, metadata)
	if err != nil {
		errorCode := status.MapGCPError(err)
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       errorCode,
				StatusMessage:   fmt.Sprintf("Failed to create BigQuery dataset: %v", err),
			},
		}, nil
	}

	nativeID := fmt.Sprintf("projects/%s/datasets/%s", project, datasetID)

	// Read back the created dataset
	readProps := flattenDataset(metadata, project, datasetID)
	readPropsJSON, _ := json.Marshal(readProps)

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			NativeID:           nativeID,
			OperationStatus:    resource.OperationStatusSuccess,
			StatusMessage:      "BigQuery dataset created successfully",
			ResourceProperties: readPropsJSON,
		},
	}, nil
}

// Read retrieves the current state of a BigQuery dataset
func (d *Dataset) Read(ctx context.Context, req *resource.ReadRequest) (*resource.ReadResult, error) {
	project, datasetID, err := parseDatasetID(req.NativeID)
	if err != nil {
		return nil, fmt.Errorf("invalid native ID: %w", err)
	}

	client, err := d.getClient(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer func() { _ = client.Close() }()

	ds := client.Dataset(datasetID)
	metadata, err := ds.Metadata(ctx)
	if err != nil {
		errorCode := status.MapGCPError(err)
		return &resource.ReadResult{
			ErrorCode: errorCode,
		}, nil
	}

	props := flattenDataset(metadata, project, datasetID)
	propsJSON, err := json.Marshal(props)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal properties: %w", err)
	}

	return &resource.ReadResult{
		Properties: string(propsJSON),
	}, nil
}

// Delete deletes a BigQuery dataset
func (d *Dataset) Delete(ctx context.Context, req *resource.DeleteRequest) (*resource.DeleteResult, error) {
	project, datasetID, err := parseDatasetID(req.NativeID)
	if err != nil {
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
				StatusMessage:   fmt.Sprintf("Invalid native ID: %v", err),
			},
		}, nil
	}

	client, err := d.getClient(ctx, project)
	if err != nil {
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeUnforeseenError,
				StatusMessage:   fmt.Sprintf("Failed to create client: %v", err),
			},
		}, nil
	}
	defer func() { _ = client.Close() }()

	ds := client.Dataset(datasetID)

	// Use DeleteWithContents to delete the dataset along with any tables/views it contains.
	// This is the expected behavior for IaC tools and handles cases where:
	// 1. Child resources were created outside of Formae management
	// 2. Dependency ordering doesn't delete children first
	err = ds.DeleteWithContents(ctx)
	if err != nil {
		errorCode := status.MapGCPError(err)
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       errorCode,
				StatusMessage:   fmt.Sprintf("Failed to delete BigQuery dataset: %v", err),
			},
		}, nil
	}

	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
			StatusMessage:   "BigQuery dataset deleted successfully",
		},
	}, nil
}

// List lists all BigQuery datasets in the project
func (d *Dataset) List(ctx context.Context, req *resource.ListRequest) (*resource.ListResult, error) {
	project := config.FromTargetConfig(req.TargetConfig).Project

	client, err := d.getClient(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer func() { _ = client.Close() }()

	it := client.Datasets(ctx)
	nativeIDs := make([]string, 0)

	for {
		ds, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list datasets: %w", err)
		}

		nativeID := fmt.Sprintf("projects/%s/datasets/%s", project, ds.DatasetID)
		nativeIDs = append(nativeIDs, nativeID)
	}

	return &resource.ListResult{
		NativeIDs: nativeIDs,
	}, nil
}

// Status is not needed for BigQuery datasets (synchronous operations)
func (d *Dataset) Status(ctx context.Context, req *resource.StatusRequest) (*resource.StatusResult, error) {
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			OperationStatus: resource.OperationStatusSuccess,
			StatusMessage:   "BigQuery dataset operations are synchronous",
		},
	}, nil
}

// Update updates the mutable metadata of a BigQuery dataset.
func (d *Dataset) Update(ctx context.Context, req *resource.UpdateRequest) (*resource.UpdateResult, error) {
	project, datasetID, err := parseDatasetID(req.NativeID)
	if err != nil {
		return bqUpdateFailure(resource.OperationErrorCodeInvalidRequest, fmt.Sprintf("Invalid native ID: %v", err)), nil
	}

	props, err := utils.ParseProperties(req.DesiredProperties)
	if err != nil {
		return bqUpdateFailure(resource.OperationErrorCodeInvalidRequest, fmt.Sprintf("Failed to parse properties: %v", err)), nil
	}

	client, err := d.getClient(ctx, project)
	if err != nil {
		return bqUpdateFailure(resource.OperationErrorCodeUnforeseenError, fmt.Sprintf("Failed to create client: %v", err)), nil
	}
	defer func() { _ = client.Close() }()

	update := bigquery.DatasetMetadataToUpdate{
		Name:        utils.GetString(props, "name"),
		Description: utils.GetString(props, "description"),
	}
	if expirationMs := utils.GetInt64(props, "defaultTableExpirationMs"); expirationMs > 0 {
		update.DefaultTableExpiration = time.Duration(expirationMs) * time.Millisecond
	}
	if partitionMs := utils.GetInt64(props, "defaultPartitionExpirationMs"); partitionMs > 0 {
		update.DefaultPartitionExpiration = time.Duration(partitionMs) * time.Millisecond
	}
	for k, v := range getStringMap(props, "labels") {
		update.SetLabel(k, v)
	}

	// etag "" => unconditional update (no optimistic-concurrency precondition)
	metadata, err := client.Dataset(datasetID).Update(ctx, update, "")
	if err != nil {
		return bqUpdateFailure(status.MapGCPError(err), fmt.Sprintf("Failed to update BigQuery dataset: %v", err)), nil
	}

	readProps := flattenDataset(metadata, project, datasetID)
	readPropsJSON, _ := json.Marshal(readProps)

	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationUpdate,
			NativeID:           req.NativeID,
			OperationStatus:    resource.OperationStatusSuccess,
			StatusMessage:      "BigQuery dataset updated successfully",
			ResourceProperties: readPropsJSON,
		},
	}, nil
}

// bqUpdateFailure builds a failed UpdateResult for BigQuery resources.
func bqUpdateFailure(code resource.OperationErrorCode, msg string) *resource.UpdateResult {
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       code,
			StatusMessage:   msg,
		},
	}
}

// Helper functions

func parseDatasetID(nativeID string) (project, datasetID string, err error) {
	// Expected format: projects/{project}/datasets/{datasetId}
	parts := make([]string, 0, 4)
	current := ""
	for _, char := range nativeID {
		if char == '/' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}

	if len(parts) != 4 || parts[0] != "projects" || parts[2] != "datasets" {
		return "", "", fmt.Errorf("invalid dataset ID format: %s", nativeID)
	}
	return parts[1], parts[3], nil
}

func flattenDataset(metadata *bigquery.DatasetMetadata, project, datasetID string) map[string]interface{} {
	props := map[string]interface{}{
		"project":      project,
		"datasetId":    datasetID,
		"name":         metadata.Name,
		"description":  metadata.Description,
		"location":     metadata.Location,
		"createdTime":  metadata.CreationTime.Unix(),
		"lastModified": metadata.LastModifiedTime.Unix(),
	}

	if metadata.Labels != nil {
		props["labels"] = metadata.Labels
	}

	if metadata.DefaultTableExpiration > 0 {
		props["defaultTableExpirationMs"] = metadata.DefaultTableExpiration.Milliseconds()
	}

	if metadata.DefaultPartitionExpiration > 0 {
		props["defaultPartitionExpirationMs"] = metadata.DefaultPartitionExpiration.Milliseconds()
	}

	return props
}

func getStringMap(m map[string]interface{}, key string) map[string]string {
	if val, ok := m[key]; ok {
		if strMap, ok := val.(map[string]interface{}); ok {
			result := make(map[string]string, len(strMap))
			for k, v := range strMap {
				if s, ok := v.(string); ok {
					result[k] = s
				}
			}
			return result
		}
	}
	return nil
}
