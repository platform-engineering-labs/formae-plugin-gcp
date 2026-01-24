// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigquery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/status"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
)

type Table struct {
	cfg *config.Config
}

var _ prov.Provisioner = &Table{}

func init() {
	registry.Register("GCP::BigQuery::Table",
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
		},
		func(cfg *config.Config) prov.Provisioner {
			return &Table{cfg: cfg}
		})
}

func (t *Table) getClient(ctx context.Context, project string) (*bigquery.Client, error) {
	opts, err := t.cfg.ToClientOptions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create client options: %w", err)
	}

	client, err := bigquery.NewClient(ctx, project, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create BigQuery client: %w", err)
	}

	return client, nil
}

// Create creates a new BigQuery table
func (t *Table) Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error) {
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
	tableID := utils.GetString(props, "tableId")

	if datasetID == "" || tableID == "" {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
				StatusMessage:   "datasetId and tableId are required",
			},
		}, nil
	}

	client, err := t.getClient(ctx, project)
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
	defer client.Close()

	table := client.Dataset(datasetID).Table(tableID)
	metadata := buildTableMetadata(props)

	err = table.Create(ctx, metadata)
	if err != nil {
		errorCode := status.MapGCPError(err)
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       errorCode,
				StatusMessage:   fmt.Sprintf("Failed to create BigQuery table: %v", err),
			},
		}, nil
	}

	nativeID := fmt.Sprintf("projects/%s/datasets/%s/tables/%s", project, datasetID, tableID)

	// Read back the created table
	readProps := flattenTable(metadata, project, datasetID, tableID)
	readPropsJSON, _ := json.Marshal(readProps)

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			NativeID:           nativeID,
			OperationStatus:    resource.OperationStatusSuccess,
			StatusMessage:      "BigQuery table created successfully",
			ResourceProperties: readPropsJSON,
		},
	}, nil
}

// Read retrieves the current state of a BigQuery table
func (t *Table) Read(ctx context.Context, req *resource.ReadRequest) (*resource.ReadResult, error) {
	project, datasetID, tableID, err := parseTableID(req.NativeID)
	if err != nil {
		return nil, fmt.Errorf("invalid native ID: %w", err)
	}

	client, err := t.getClient(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	table := client.Dataset(datasetID).Table(tableID)
	metadata, err := table.Metadata(ctx)
	if err != nil {
		errorCode := status.MapGCPError(err)
		return &resource.ReadResult{
			ErrorCode: errorCode,
		}, nil
	}

	props := flattenTable(metadata, project, datasetID, tableID)
	propsJSON, err := json.Marshal(props)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal properties: %w", err)
	}

	return &resource.ReadResult{
		Properties: string(propsJSON),
	}, nil
}

// Delete deletes a BigQuery table
func (t *Table) Delete(ctx context.Context, req *resource.DeleteRequest) (*resource.DeleteResult, error) {
	project, datasetID, tableID, err := parseTableID(req.NativeID)
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

	client, err := t.getClient(ctx, project)
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
	defer client.Close()

	table := client.Dataset(datasetID).Table(tableID)
	err = table.Delete(ctx)
	if err != nil {
		errorCode := status.MapGCPError(err)
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       errorCode,
				StatusMessage:   fmt.Sprintf("Failed to delete BigQuery table: %v", err),
			},
		}, nil
	}

	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
			StatusMessage:   "BigQuery table deleted successfully",
		},
	}, nil
}

// List lists all BigQuery tables in a dataset
func (t *Table) List(ctx context.Context, req *resource.ListRequest) (*resource.ListResult, error) {
	project := config.FromTargetConfig(req.TargetConfig).Project

	// Get dataset ID from additional properties
	datasetID := ""
	if req.AdditionalProperties != nil {
		datasetID = req.AdditionalProperties["datasetId"]
	}

	if datasetID == "" {
		return nil, fmt.Errorf("datasetId must be provided in AdditionalProperties for listing tables")
	}

	client, err := t.getClient(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	it := client.Dataset(datasetID).Tables(ctx)
	nativeIDs := make([]string, 0)

	for {
		table, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list tables: %w", err)
		}

		nativeID := fmt.Sprintf("projects/%s/datasets/%s/tables/%s", project, datasetID, table.TableID)
		nativeIDs = append(nativeIDs, nativeID)
	}

	return &resource.ListResult{
		NativeIDs: nativeIDs,
	}, nil
}

// Status is not needed for BigQuery tables (synchronous operations)
func (t *Table) Status(ctx context.Context, req *resource.StatusRequest) (*resource.StatusResult, error) {
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			OperationStatus: resource.OperationStatusSuccess,
			StatusMessage:   "BigQuery table operations are synchronous",
		},
	}, nil
}

// Update is not implemented
func (t *Table) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       resource.OperationErrorCodeNotUpdatable,
			StatusMessage:   "Update not implemented for BigQuery table",
		},
	}, nil
}

// Helper functions

func parseTableID(nativeID string) (project, datasetID, tableID string, err error) {
	// Expected format: projects/{project}/datasets/{dataset}/tables/{table}
	parts := strings.Split(nativeID, "/")
	if len(parts) != 6 || parts[0] != "projects" || parts[2] != "datasets" || parts[4] != "tables" {
		return "", "", "", fmt.Errorf("invalid table ID format: %s", nativeID)
	}
	return parts[1], parts[3], parts[5], nil
}

func buildTableMetadata(props map[string]interface{}) *bigquery.TableMetadata {
	metadata := &bigquery.TableMetadata{
		Name:        utils.GetString(props, "name"),
		Description: utils.GetString(props, "description"),
		Labels:      getStringMap(props, "labels"),
	}

	// Schema
	if schemaFields := getSchemaFields(props, "schema"); schemaFields != nil {
		metadata.Schema = schemaFields
	}

	// View
	if viewQuery := utils.GetString(props, "view"); viewQuery != "" {
		metadata.ViewQuery = viewQuery
	}

	// Materialized view
	if mvQuery := utils.GetString(props, "materializedView"); mvQuery != "" {
		metadata.MaterializedView = &bigquery.MaterializedViewDefinition{
			Query: mvQuery,
		}
	}

	// External data configuration
	if externalData := utils.GetObject(props, "externalDataConfiguration"); externalData != nil {
		metadata.ExternalDataConfig = buildExternalDataConfig(externalData)
	}

	// Time partitioning
	if timePartitioning := utils.GetObject(props, "timePartitioning"); timePartitioning != nil {
		metadata.TimePartitioning = &bigquery.TimePartitioning{
			Type:  bigquery.TimePartitioningType(utils.GetString(timePartitioning, "type")),
			Field: utils.GetString(timePartitioning, "field"),
		}
		if expirationMs := utils.GetInt64(timePartitioning, "expirationMs"); expirationMs > 0 {
			metadata.TimePartitioning.Expiration = time.Duration(expirationMs) * time.Millisecond
		}
	}

	// Range partitioning
	if rangePartitioning := utils.GetObject(props, "rangePartitioning"); rangePartitioning != nil {
		metadata.RangePartitioning = &bigquery.RangePartitioning{
			Field: utils.GetString(rangePartitioning, "field"),
		}
		if rangeConfig := utils.GetObject(rangePartitioning, "range"); rangeConfig != nil {
			metadata.RangePartitioning.Range = &bigquery.RangePartitioningRange{
				Start:    utils.GetInt64(rangeConfig, "start"),
				End:      utils.GetInt64(rangeConfig, "end"),
				Interval: utils.GetInt64(rangeConfig, "interval"),
			}
		}
	}

	// Clustering
	if clustering := getStringArray(props, "clustering"); clustering != nil {
		metadata.Clustering = &bigquery.Clustering{
			Fields: clustering,
		}
	}

	// Expiration (TableMetadata.ExpirationTime is time.Time, not duration)
	// Skip for now as it requires proper time conversion

	// Encryption
	if encryption := utils.GetObject(props, "encryptionConfiguration"); encryption != nil {
		metadata.EncryptionConfig = &bigquery.EncryptionConfig{
			KMSKeyName: utils.GetString(encryption, "kmsKeyName"),
		}
	}

	return metadata
}

func buildExternalDataConfig(props map[string]interface{}) *bigquery.ExternalDataConfig {
	config := &bigquery.ExternalDataConfig{
		SourceFormat: bigquery.DataFormat(utils.GetString(props, "sourceFormat")),
		SourceURIs:   getStringArray(props, "sourceUris"),
		AutoDetect:   utils.GetBool(props, "autodetect"),
	}

	// CSV options
	if csvOptions := utils.GetObject(props, "csvOptions"); csvOptions != nil {
		csvOpts := &bigquery.CSVOptions{
			FieldDelimiter:      utils.GetString(csvOptions, "fieldDelimiter"),
			SkipLeadingRows:     utils.GetInt64(csvOptions, "skipLeadingRows"),
			Quote:               utils.GetString(csvOptions, "quote"),
			AllowJaggedRows:     utils.GetBool(csvOptions, "allowJaggedRows"),
			AllowQuotedNewlines: utils.GetBool(csvOptions, "allowQuotedNewlines"),
		}
		if encoding := utils.GetString(csvOptions, "encoding"); encoding != "" {
			csvOpts.Encoding = bigquery.Encoding(encoding)
		}
		config.Options = csvOpts
	}

	// Google Sheets options
	if sheetsOptions := utils.GetObject(props, "googleSheetsOptions"); sheetsOptions != nil {
		config.Options = &bigquery.GoogleSheetsOptions{
			SkipLeadingRows: utils.GetInt64(sheetsOptions, "skipLeadingRows"),
			Range:           utils.GetString(sheetsOptions, "range"),
		}
	}

	return config
}

func getSchemaFields(props map[string]interface{}, key string) bigquery.Schema {
	if val, ok := props[key]; ok {
		if arr, ok := val.([]interface{}); ok {
			schema := make(bigquery.Schema, 0, len(arr))
			for _, item := range arr {
				if obj, ok := item.(map[string]interface{}); ok {
					field := buildSchemaField(obj)
					schema = append(schema, field)
				}
			}
			return schema
		}
	}
	return nil
}

func buildSchemaField(obj map[string]interface{}) *bigquery.FieldSchema {
	field := &bigquery.FieldSchema{
		Name:        utils.GetString(obj, "name"),
		Description: utils.GetString(obj, "description"),
		Type:        bigquery.FieldType(utils.GetString(obj, "type")),
		Required:    utils.GetBool(obj, "required"),
	}

	// Mode
	if mode := utils.GetString(obj, "mode"); mode != "" {
		field.Repeated = mode == "REPEATED"
		field.Required = mode == "REQUIRED"
	}

	// Nested fields (for RECORD/STRUCT types)
	if fields := getSchemaFields(obj, "fields"); fields != nil {
		field.Schema = fields
	}

	return field
}

func flattenTable(metadata *bigquery.TableMetadata, project, datasetID, tableID string) map[string]interface{} {
	props := map[string]interface{}{
		"project":      project,
		"datasetId":    datasetID,
		"tableId":      tableID,
		"name":         metadata.Name,
		"description":  metadata.Description,
		"type":         string(metadata.Type),
		"createdTime":  metadata.CreationTime.Unix(),
		"lastModified": metadata.LastModifiedTime.Unix(),
		"numBytes":     metadata.NumBytes,
		"numRows":      metadata.NumRows,
	}

	if metadata.Labels != nil {
		props["labels"] = metadata.Labels
	}

	if metadata.Schema != nil {
		props["schema"] = flattenSchema(metadata.Schema)
	}

	if metadata.ViewQuery != "" {
		props["view"] = metadata.ViewQuery
	}

	if metadata.MaterializedView != nil {
		props["materializedView"] = metadata.MaterializedView.Query
	}

	if metadata.TimePartitioning != nil {
		props["timePartitioning"] = map[string]interface{}{
			"type":  string(metadata.TimePartitioning.Type),
			"field": metadata.TimePartitioning.Field,
		}
	}

	if metadata.Clustering != nil {
		props["clustering"] = metadata.Clustering.Fields
	}

	if metadata.EncryptionConfig != nil {
		props["encryptionConfiguration"] = map[string]interface{}{
			"kmsKeyName": metadata.EncryptionConfig.KMSKeyName,
		}
	}

	return props
}

func flattenSchema(schema bigquery.Schema) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(schema))
	for _, field := range schema {
		fieldMap := map[string]interface{}{
			"name":        field.Name,
			"type":        string(field.Type),
			"description": field.Description,
		}

		if field.Required {
			fieldMap["mode"] = "REQUIRED"
		} else if field.Repeated {
			fieldMap["mode"] = "REPEATED"
		} else {
			fieldMap["mode"] = "NULLABLE"
		}

		if field.Schema != nil {
			fieldMap["fields"] = flattenSchema(field.Schema)
		}

		result = append(result, fieldMap)
	}
	return result
}

func getStringArray(m map[string]interface{}, key string) []string {
	if val, ok := m[key]; ok {
		if arr, ok := val.([]interface{}); ok {
			result := make([]string, 0, len(arr))
			for _, item := range arr {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
	}
	return nil
}
