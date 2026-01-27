// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigquery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/status"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

type Routine struct {
	cfg *config.Config
}

var _ prov.Provisioner = &Routine{}

func init() {
	registry.Register("GCP::BigQuery::Routine",
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
		},
		func(cfg *config.Config) prov.Provisioner {
			return &Routine{cfg: cfg}
		})
}

func (r *Routine) getClient(ctx context.Context, project string) (*bigquery.Client, error) {
	opts, err := r.cfg.ToClientOptions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create client options: %w", err)
	}

	client, err := bigquery.NewClient(ctx, project, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create BigQuery client: %w", err)
	}

	return client, nil
}

// Create creates a new BigQuery routine
func (r *Routine) Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error) {
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
	routineID := utils.GetString(props, "routineId")

	if datasetID == "" || routineID == "" {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
				StatusMessage:   "datasetId and routineId are required",
			},
		}, nil
	}

	client, err := r.getClient(ctx, project)
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

	routine := client.Dataset(datasetID).Routine(routineID)
	metadata := buildRoutineMetadata(props)

	err = routine.Create(ctx, metadata)
	if err != nil {
		errorCode := status.MapGCPError(err)
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       errorCode,
				StatusMessage:   fmt.Sprintf("Failed to create BigQuery routine: %v", err),
			},
		}, nil
	}

	nativeID := fmt.Sprintf("projects/%s/datasets/%s/routines/%s", project, datasetID, routineID)

	// Read back the created routine
	readProps := flattenRoutine(metadata, project, datasetID, routineID)
	readPropsJSON, _ := json.Marshal(readProps)

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			NativeID:           nativeID,
			OperationStatus:    resource.OperationStatusSuccess,
			StatusMessage:      "BigQuery routine created successfully",
			ResourceProperties: readPropsJSON,
		},
	}, nil
}

// Read retrieves the current state of a BigQuery routine
func (r *Routine) Read(ctx context.Context, req *resource.ReadRequest) (*resource.ReadResult, error) {
	project, datasetID, routineID, err := parseRoutineID(req.NativeID)
	if err != nil {
		return nil, fmt.Errorf("invalid native ID: %w", err)
	}

	client, err := r.getClient(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	routine := client.Dataset(datasetID).Routine(routineID)
	metadata, err := routine.Metadata(ctx)
	if err != nil {
		errorCode := status.MapGCPError(err)
		return &resource.ReadResult{
			ErrorCode: errorCode,
		}, nil
	}

	props := flattenRoutine(metadata, project, datasetID, routineID)
	propsJSON, err := json.Marshal(props)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal properties: %w", err)
	}

	return &resource.ReadResult{
		Properties: string(propsJSON),
	}, nil
}

// Delete deletes a BigQuery routine
func (r *Routine) Delete(ctx context.Context, req *resource.DeleteRequest) (*resource.DeleteResult, error) {
	project, datasetID, routineID, err := parseRoutineID(req.NativeID)
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

	client, err := r.getClient(ctx, project)
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

	routine := client.Dataset(datasetID).Routine(routineID)
	err = routine.Delete(ctx)
	if err != nil {
		errorCode := status.MapGCPError(err)
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       errorCode,
				StatusMessage:   fmt.Sprintf("Failed to delete BigQuery routine: %v", err),
			},
		}, nil
	}

	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
			StatusMessage:   "BigQuery routine deleted successfully",
		},
	}, nil
}

// List lists all BigQuery routines in a dataset
func (r *Routine) List(ctx context.Context, req *resource.ListRequest) (*resource.ListResult, error) {
	project := config.FromTargetConfig(req.TargetConfig).Project

	// Get dataset ID from additional properties
	datasetID := ""
	if req.AdditionalProperties != nil {
		datasetID = req.AdditionalProperties["datasetId"]
	}

	if datasetID == "" {
		return nil, fmt.Errorf("datasetId must be provided in AdditionalProperties for listing routines")
	}

	client, err := r.getClient(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	it := client.Dataset(datasetID).Routines(ctx)
	nativeIDs := make([]string, 0)

	for {
		routine, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list routines: %w", err)
		}

		nativeID := fmt.Sprintf("projects/%s/datasets/%s/routines/%s", project, datasetID, routine.RoutineID)
		nativeIDs = append(nativeIDs, nativeID)
	}

	return &resource.ListResult{
		NativeIDs: nativeIDs,
	}, nil
}

// Status is not needed for BigQuery routines (synchronous operations)
func (r *Routine) Status(ctx context.Context, req *resource.StatusRequest) (*resource.StatusResult, error) {
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			OperationStatus: resource.OperationStatusSuccess,
			StatusMessage:   "BigQuery routine operations are synchronous",
		},
	}, nil
}

// Update is not implemented
func (r *Routine) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       resource.OperationErrorCodeNotUpdatable,
			StatusMessage:   "Update not implemented for BigQuery routine",
		},
	}, nil
}

// Helper functions

func parseRoutineID(nativeID string) (project, datasetID, routineID string, err error) {
	// Expected format: projects/{project}/datasets/{dataset}/routines/{routine}
	parts := strings.Split(nativeID, "/")
	if len(parts) != 6 || parts[0] != "projects" || parts[2] != "datasets" || parts[4] != "routines" {
		return "", "", "", fmt.Errorf("invalid routine ID format: %s", nativeID)
	}
	return parts[1], parts[3], parts[5], nil
}

func buildRoutineMetadata(props map[string]interface{}) *bigquery.RoutineMetadata {
	metadata := &bigquery.RoutineMetadata{
		Type:        utils.GetString(props, "type"),
		Language:    utils.GetString(props, "language"),
		Description: utils.GetString(props, "description"),
		Body:        utils.GetString(props, "body"),
	}

	// Determinism level
	if determinism := utils.GetString(props, "determinismLevel"); determinism != "" {
		metadata.DeterminismLevel = bigquery.RoutineDeterminism(determinism)
	}

	// Arguments
	if args := getRoutineArguments(props, "arguments"); args != nil {
		metadata.Arguments = args
	}

	// Return type
	if returnType := utils.GetObject(props, "returnType"); returnType != nil {
		metadata.ReturnType = &bigquery.StandardSQLDataType{
			TypeKind: utils.GetString(returnType, "typeKind"),
		}
	}

	// Return table type (for table-valued functions)
	if returnTableType := utils.GetObject(props, "returnTableType"); returnTableType != nil {
		// Build table schema for return type
		if columns := getRoutineColumns(returnTableType, "columns"); columns != nil {
			metadata.ReturnTableType = &bigquery.StandardSQLTableType{
				Columns: columns,
			}
		}
	}

	// Imported libraries
	if libraries := getStringArrayFromProps(props, "importedLibraries"); libraries != nil {
		metadata.ImportedLibraries = libraries
	}

	return metadata
}

func getRoutineArguments(props map[string]interface{}, key string) []*bigquery.RoutineArgument {
	if val, ok := props[key]; ok {
		if arr, ok := val.([]interface{}); ok {
			args := make([]*bigquery.RoutineArgument, 0, len(arr))
			for _, item := range arr {
				if obj, ok := item.(map[string]interface{}); ok {
					arg := &bigquery.RoutineArgument{
						Name: utils.GetString(obj, "name"),
						Kind: utils.GetString(obj, "kind"),
					}
					if dataType := utils.GetObject(obj, "dataType"); dataType != nil {
						arg.DataType = &bigquery.StandardSQLDataType{
							TypeKind: utils.GetString(dataType, "typeKind"),
						}
					}
					args = append(args, arg)
				}
			}
			return args
		}
	}
	return nil
}

func getRoutineColumns(props map[string]interface{}, key string) []*bigquery.StandardSQLField {
	if val, ok := props[key]; ok {
		if arr, ok := val.([]interface{}); ok {
			columns := make([]*bigquery.StandardSQLField, 0, len(arr))
			for _, item := range arr {
				if obj, ok := item.(map[string]interface{}); ok {
					column := &bigquery.StandardSQLField{
						Name: utils.GetString(obj, "name"),
					}
					if dataType := utils.GetObject(obj, "type"); dataType != nil {
						column.Type = &bigquery.StandardSQLDataType{
							TypeKind: utils.GetString(dataType, "typeKind"),
						}
					}
					columns = append(columns, column)
				}
			}
			return columns
		}
	}
	return nil
}

func flattenRoutine(metadata *bigquery.RoutineMetadata, project, datasetID, routineID string) map[string]interface{} {
	props := map[string]interface{}{
		"project":      project,
		"datasetId":    datasetID,
		"routineId":    routineID,
		"type":         string(metadata.Type),
		"language":     metadata.Language,
		"description":  metadata.Description,
		"body":         metadata.Body,
		"createdTime":  metadata.CreationTime.Unix(),
		"lastModified": metadata.LastModifiedTime.Unix(),
	}

	if metadata.DeterminismLevel != "" {
		props["determinismLevel"] = string(metadata.DeterminismLevel)
	}

	if metadata.Arguments != nil && len(metadata.Arguments) > 0 {
		args := make([]map[string]interface{}, 0, len(metadata.Arguments))
		for _, arg := range metadata.Arguments {
			argMap := map[string]interface{}{
				"name": arg.Name,
				"kind": string(arg.Kind),
			}
			if arg.DataType != nil {
				argMap["dataType"] = map[string]interface{}{
					"typeKind": string(arg.DataType.TypeKind),
				}
			}
			args = append(args, argMap)
		}
		props["arguments"] = args
	}

	if metadata.ReturnType != nil {
		props["returnType"] = map[string]interface{}{
			"typeKind": string(metadata.ReturnType.TypeKind),
		}
	}

	if metadata.ReturnTableType != nil && len(metadata.ReturnTableType.Columns) > 0 {
		columns := make([]map[string]interface{}, 0, len(metadata.ReturnTableType.Columns))
		for _, col := range metadata.ReturnTableType.Columns {
			colMap := map[string]interface{}{
				"name": col.Name,
			}
			if col.Type != nil {
				colMap["type"] = map[string]interface{}{
					"typeKind": string(col.Type.TypeKind),
				}
			}
			columns = append(columns, colMap)
		}
		props["returnTableType"] = map[string]interface{}{
			"columns": columns,
		}
	}

	if metadata.ImportedLibraries != nil && len(metadata.ImportedLibraries) > 0 {
		props["importedLibraries"] = metadata.ImportedLibraries
	}

	return props
}

func getStringArrayFromProps(m map[string]interface{}, key string) []string {
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
