// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration

package bigquery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/testutil"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	RoutineResourceType = "GCP::BigQuery::Routine"
)

// createTestDatasetForRoutine creates a test dataset and returns the dataset ID and a cleanup function.
func createTestDatasetForRoutine(t *testing.T, ctx context.Context) (string, func()) {
	t.Helper()

	datasetProv := &Dataset{cfg: testutil.Config}
	datasetID := fmt.Sprintf("formae_routine_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	datasetProps := map[string]interface{}{
		"datasetId": datasetID,
		"location":  testutil.Region,
	}
	propsJSON, err := json.Marshal(datasetProps)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: "GCP::BigQuery::Dataset",
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := datasetProv.Create(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

	cleanup := func() {
		datasetNativeID := fmt.Sprintf("projects/%s/datasets/%s", testutil.Project, datasetID)
		deleteReq := &resource.DeleteRequest{
			NativeID:     datasetNativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = datasetProv.Delete(ctx, deleteReq)
	}

	return datasetID, cleanup
}

// TestRoutine_Create_Integration tests creating a SQL UDF routine.
func TestRoutine_Create_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	datasetID, cleanupDataset := createTestDatasetForRoutine(t, ctx)
	defer cleanupDataset()

	routineProv := &Routine{cfg: testutil.Config}
	routineID := fmt.Sprintf("test_udf_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	routineProperties := map[string]interface{}{
		"datasetId":   datasetID,
		"routineId":   routineID,
		"type":        "SCALAR_FUNCTION",
		"language":    "SQL",
		"description": "Test SQL UDF",
		"body":        "x * 2",
		"arguments": []map[string]interface{}{
			{
				"name": "x",
				"dataType": map[string]interface{}{
					"typeKind": "INT64",
				},
			},
		},
		"returnType": map[string]interface{}{
			"typeKind": "INT64",
		},
	}

	propsJSON, err := json.Marshal(routineProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: RoutineResourceType,
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := routineProv.Create(ctx, createReq)
	require.NoError(t, err)
	require.NotNil(t, createResult)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)
	require.NotEmpty(t, createResult.ProgressResult.NativeID)

	nativeID := createResult.ProgressResult.NativeID
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = routineProv.Delete(ctx, deleteReq)
	}()

	// Verify native ID format
	expectedPrefix := fmt.Sprintf("projects/%s/datasets/%s/routines/%s", testutil.Project, datasetID, routineID)
	assert.Equal(t, expectedPrefix, nativeID)
	t.Logf("Routine created with native ID: %s", nativeID)
}

// TestRoutine_Read_Integration tests reading a routine.
func TestRoutine_Read_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	datasetID, cleanupDataset := createTestDatasetForRoutine(t, ctx)
	defer cleanupDataset()

	routineProv := &Routine{cfg: testutil.Config}
	routineID := fmt.Sprintf("test_udf_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	// Create routine first
	routineProperties := map[string]interface{}{
		"datasetId":   datasetID,
		"routineId":   routineID,
		"type":        "SCALAR_FUNCTION",
		"language":    "SQL",
		"description": "Test SQL UDF for read",
		"body":        "x * 2",
		"arguments": []map[string]interface{}{
			{
				"name": "x",
				"dataType": map[string]interface{}{
					"typeKind": "INT64",
				},
			},
		},
		"returnType": map[string]interface{}{
			"typeKind": "INT64",
		},
	}

	propsJSON, err := json.Marshal(routineProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: RoutineResourceType,
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := routineProv.Create(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

	nativeID := createResult.ProgressResult.NativeID
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = routineProv.Delete(ctx, deleteReq)
	}()

	// Read the routine
	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := routineProv.Read(ctx, readReq)
	require.NoError(t, err)
	require.NotNil(t, readResult)
	require.NotEmpty(t, readResult.Properties)

	// Unmarshal and verify properties
	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err)

	assert.Equal(t, routineID, props["routineId"])
	assert.Equal(t, datasetID, props["datasetId"])
	assert.Equal(t, "SCALAR_FUNCTION", props["type"])
	assert.Equal(t, "SQL", props["language"])
	assert.Equal(t, "x * 2", props["body"])

	// Verify arguments
	if args, ok := props["arguments"].([]interface{}); ok {
		assert.Len(t, args, 1)
		if arg, ok := args[0].(map[string]interface{}); ok {
			assert.Equal(t, "x", arg["name"])
		}
	}
	t.Logf("Routine read successfully")
}

// TestRoutine_Update_NotUpdatable_Integration tests that routine updates return NotUpdatable.
func TestRoutine_Update_NotUpdatable_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	datasetID, cleanupDataset := createTestDatasetForRoutine(t, ctx)
	defer cleanupDataset()

	routineProv := &Routine{cfg: testutil.Config}
	routineID := fmt.Sprintf("test_udf_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	// Create routine first
	routineProperties := map[string]interface{}{
		"datasetId":   datasetID,
		"routineId":   routineID,
		"type":        "SCALAR_FUNCTION",
		"language":    "SQL",
		"description": "Test SQL UDF",
		"body":        "x * 2",
		"arguments": []map[string]interface{}{
			{
				"name": "x",
				"dataType": map[string]interface{}{
					"typeKind": "INT64",
				},
			},
		},
		"returnType": map[string]interface{}{
			"typeKind": "INT64",
		},
	}

	propsJSON, err := json.Marshal(routineProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: RoutineResourceType,
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := routineProv.Create(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

	nativeID := createResult.ProgressResult.NativeID
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = routineProv.Delete(ctx, deleteReq)
	}()

	// Attempt to update
	updatedProperties := map[string]interface{}{
		"datasetId":   datasetID,
		"routineId":   routineID,
		"description": "Updated description",
	}

	updatedPropsJSON, err := json.Marshal(updatedProperties)
	require.NoError(t, err)

	updateReq := &resource.UpdateRequest{
		NativeID:          nativeID,
		ResourceType:      RoutineResourceType,
		DesiredProperties: updatedPropsJSON,
		TargetConfig:      testutil.TargetConfig,
	}

	updateResult, err := routineProv.Update(ctx, updateReq)
	require.NoError(t, err)
	require.NotNil(t, updateResult)
	require.Equal(t, resource.OperationStatusFailure, updateResult.ProgressResult.OperationStatus)
	require.Equal(t, resource.OperationErrorCodeNotUpdatable, updateResult.ProgressResult.ErrorCode)
	t.Logf("Update correctly returned NotUpdatable status")
}

// TestRoutine_Delete_Integration tests deleting a routine.
func TestRoutine_Delete_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	datasetID, cleanupDataset := createTestDatasetForRoutine(t, ctx)
	defer cleanupDataset()

	routineProv := &Routine{cfg: testutil.Config}
	routineID := fmt.Sprintf("test_udf_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	// Create routine first
	routineProperties := map[string]interface{}{
		"datasetId":   datasetID,
		"routineId":   routineID,
		"type":        "SCALAR_FUNCTION",
		"language":    "SQL",
		"body":        "x * 2",
		"arguments": []map[string]interface{}{
			{
				"name": "x",
				"dataType": map[string]interface{}{
					"typeKind": "INT64",
				},
			},
		},
		"returnType": map[string]interface{}{
			"typeKind": "INT64",
		},
	}

	propsJSON, err := json.Marshal(routineProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: RoutineResourceType,
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := routineProv.Create(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

	nativeID := createResult.ProgressResult.NativeID

	// Delete the routine
	deleteReq := &resource.DeleteRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
	}

	deleteResult, err := routineProv.Delete(ctx, deleteReq)
	require.NoError(t, err)
	require.NotNil(t, deleteResult)
	require.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus)
	t.Logf("Routine deleted successfully: %s", nativeID)

	// Verify it's actually deleted by reading
	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := routineProv.Read(ctx, readReq)
	require.NoError(t, err)
	require.NotNil(t, readResult)
	require.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode)
	t.Logf("Verified routine is deleted (not found)")
}

// TestRoutine_Delete_NotFound_Integration tests deleting a non-existent routine.
func TestRoutine_Delete_NotFound_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	routineProv := &Routine{cfg: testutil.Config}

	// Try to delete a non-existent routine
	nonExistentID := fmt.Sprintf("projects/%s/datasets/nonexistent_dataset/routines/nonexistent_routine", testutil.Project)

	deleteReq := &resource.DeleteRequest{
		NativeID:     nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	deleteResult, err := routineProv.Delete(ctx, deleteReq)
	require.NoError(t, err)
	require.NotNil(t, deleteResult)
	// BigQuery returns Failure for non-existent resources (not idempotent)
	require.Equal(t, resource.OperationStatusFailure, deleteResult.ProgressResult.OperationStatus)
	t.Logf("Delete non-existent routine returned status: %s", deleteResult.ProgressResult.OperationStatus)
}

// TestRoutine_List_Integration tests listing routines.
func TestRoutine_List_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	datasetID, cleanupDataset := createTestDatasetForRoutine(t, ctx)
	defer cleanupDataset()

	routineProv := &Routine{cfg: testutil.Config}
	routineID := fmt.Sprintf("test_udf_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	// Create routine first
	routineProperties := map[string]interface{}{
		"datasetId":   datasetID,
		"routineId":   routineID,
		"type":        "SCALAR_FUNCTION",
		"language":    "SQL",
		"body":        "x * 2",
		"arguments": []map[string]interface{}{
			{
				"name": "x",
				"dataType": map[string]interface{}{
					"typeKind": "INT64",
				},
			},
		},
		"returnType": map[string]interface{}{
			"typeKind": "INT64",
		},
	}

	propsJSON, err := json.Marshal(routineProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: RoutineResourceType,
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := routineProv.Create(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

	nativeID := createResult.ProgressResult.NativeID
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = routineProv.Delete(ctx, deleteReq)
	}()

	// List routines
	listReq := &resource.ListRequest{
		ResourceType: RoutineResourceType,
		TargetConfig: testutil.TargetConfig,
		AdditionalProperties: map[string]string{
			"datasetId": datasetID,
		},
	}

	listResult, err := routineProv.List(ctx, listReq)
	require.NoError(t, err)
	require.NotNil(t, listResult)
	require.NotEmpty(t, listResult.NativeIDs)
	t.Logf("Listed %d routines", len(listResult.NativeIDs))

	// Verify our routine is in the list
	found := false
	for _, id := range listResult.NativeIDs {
		if id == nativeID {
			found = true
			break
		}
	}
	assert.True(t, found, "Created routine should be in the list")
}

// TestRoutine_CreateJavaScriptUDF_Integration tests creating a JavaScript UDF.
func TestRoutine_CreateJavaScriptUDF_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	datasetID, cleanupDataset := createTestDatasetForRoutine(t, ctx)
	defer cleanupDataset()

	routineProv := &Routine{cfg: testutil.Config}
	routineID := fmt.Sprintf("js_udf_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	routineProperties := map[string]interface{}{
		"datasetId":   datasetID,
		"routineId":   routineID,
		"type":        "SCALAR_FUNCTION",
		"language":    "JAVASCRIPT",
		"description": "Test JavaScript UDF",
		"body":        "return x.toUpperCase();",
		"arguments": []map[string]interface{}{
			{
				"name": "x",
				"dataType": map[string]interface{}{
					"typeKind": "STRING",
				},
			},
		},
		"returnType": map[string]interface{}{
			"typeKind": "STRING",
		},
	}

	propsJSON, err := json.Marshal(routineProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: RoutineResourceType,
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := routineProv.Create(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

	nativeID := createResult.ProgressResult.NativeID
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = routineProv.Delete(ctx, deleteReq)
	}()

	t.Logf("JavaScript UDF created: %s", nativeID)

	// Read and verify
	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := routineProv.Read(ctx, readReq)
	require.NoError(t, err)

	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err)

	assert.Equal(t, "JAVASCRIPT", props["language"])
	assert.Contains(t, props["body"], "toUpperCase")
	t.Logf("JavaScript UDF verified")
}

// TestRoutine_CreateStoredProcedure_Integration tests creating a stored procedure.
func TestRoutine_CreateStoredProcedure_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	datasetID, cleanupDataset := createTestDatasetForRoutine(t, ctx)
	defer cleanupDataset()

	routineProv := &Routine{cfg: testutil.Config}
	routineID := fmt.Sprintf("test_proc_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	routineProperties := map[string]interface{}{
		"datasetId":   datasetID,
		"routineId":   routineID,
		"type":        "PROCEDURE",
		"language":    "SQL",
		"description": "Test stored procedure",
		"body":        "BEGIN\n  SELECT 1;\nEND;",
	}

	propsJSON, err := json.Marshal(routineProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: RoutineResourceType,
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := routineProv.Create(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

	nativeID := createResult.ProgressResult.NativeID
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = routineProv.Delete(ctx, deleteReq)
	}()

	t.Logf("Stored procedure created: %s", nativeID)

	// Read and verify
	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := routineProv.Read(ctx, readReq)
	require.NoError(t, err)

	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err)

	assert.Equal(t, "PROCEDURE", props["type"])
	assert.Contains(t, props["body"], "BEGIN")
	t.Logf("Stored procedure verified")
}

// TestRoutine_CreateTableValuedFunction_Integration tests creating a table-valued function.
func TestRoutine_CreateTableValuedFunction_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	datasetID, cleanupDataset := createTestDatasetForRoutine(t, ctx)
	defer cleanupDataset()

	routineProv := &Routine{cfg: testutil.Config}
	routineID := fmt.Sprintf("tvf_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	routineProperties := map[string]interface{}{
		"datasetId":   datasetID,
		"routineId":   routineID,
		"type":        "TABLE_VALUED_FUNCTION",
		"language":    "SQL",
		"description": "Test table-valued function",
		"body":        "SELECT id, name FROM UNNEST([STRUCT(1 AS id, 'test' AS name)]) WHERE id > min_id",
		"arguments": []map[string]interface{}{
			{
				"name": "min_id",
				"dataType": map[string]interface{}{
					"typeKind": "INT64",
				},
			},
		},
		"returnTableType": map[string]interface{}{
			"columns": []map[string]interface{}{
				{
					"name": "id",
					"type": map[string]interface{}{
						"typeKind": "INT64",
					},
				},
				{
					"name": "name",
					"type": map[string]interface{}{
						"typeKind": "STRING",
					},
				},
			},
		},
	}

	propsJSON, err := json.Marshal(routineProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: RoutineResourceType,
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := routineProv.Create(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

	nativeID := createResult.ProgressResult.NativeID
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = routineProv.Delete(ctx, deleteReq)
	}()

	t.Logf("Table-valued function created: %s", nativeID)

	// Read and verify
	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := routineProv.Read(ctx, readReq)
	require.NoError(t, err)

	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err)

	assert.Equal(t, "TABLE_VALUED_FUNCTION", props["type"])

	// Verify return table type
	if returnTableType, ok := props["returnTableType"].(map[string]interface{}); ok {
		if columns, ok := returnTableType["columns"].([]interface{}); ok {
			assert.Len(t, columns, 2)
			t.Logf("Table-valued function return columns verified")
		}
	}
}

// TestRoutine_ListWithoutDatasetID_Integration tests that listing without datasetId fails.
func TestRoutine_ListWithoutDatasetID_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	routineProv := &Routine{cfg: testutil.Config}

	listReq := &resource.ListRequest{
		ResourceType: RoutineResourceType,
		TargetConfig: testutil.TargetConfig,
		// Missing AdditionalProperties with datasetId
	}

	_, err := routineProv.List(ctx, listReq)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "datasetId")
	t.Logf("Correctly rejected list without datasetId")
}

// TestRoutine_CreateInvalid_Integration tests error handling for invalid create requests.
func TestRoutine_CreateInvalid_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	routineProv := &Routine{cfg: testutil.Config}

	// Test with missing datasetId
	invalidProperties := map[string]interface{}{
		"routineId": "test_routine",
		"type":      "SCALAR_FUNCTION",
		"language":  "SQL",
		"body":      "1 + 1",
	}

	propsJSON, err := json.Marshal(invalidProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: RoutineResourceType,
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := routineProv.Create(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusFailure, createResult.ProgressResult.OperationStatus)
	assert.Contains(t, createResult.ProgressResult.StatusMessage, "required")
	t.Logf("Correctly rejected create with missing datasetId")

	// Test with missing routineId
	invalidProperties2 := map[string]interface{}{
		"datasetId": "test_dataset",
		"type":      "SCALAR_FUNCTION",
		"language":  "SQL",
		"body":      "1 + 1",
	}

	propsJSON2, err := json.Marshal(invalidProperties2)
	require.NoError(t, err)

	createReq2 := &resource.CreateRequest{
		ResourceType: RoutineResourceType,
		Properties:   propsJSON2,
		TargetConfig: testutil.TargetConfig,
	}

	createResult2, err := routineProv.Create(ctx, createReq2)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusFailure, createResult2.ProgressResult.OperationStatus)
	assert.Contains(t, createResult2.ProgressResult.StatusMessage, "required")
	t.Logf("Correctly rejected create with missing routineId")
}
