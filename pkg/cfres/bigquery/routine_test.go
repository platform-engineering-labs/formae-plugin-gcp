// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration
// +build integration

package bigquery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	RoutineResourceType = "GCP::BigQuery::Routine"
)

// TestRoutineCreateUDF tests creating a SQL UDF
func TestRoutineCreateUDF(t *testing.T) {
	datasetProv := &Dataset{cfg: testutil.Config}
	routineProv := &Routine{cfg: testutil.Config}

	datasetID := fmt.Sprintf("formae_routine_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))
	routineID := fmt.Sprintf("test_udf_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))
	ctx := context.Background()
	nativeID := ""

	// Setup: Create dataset
	t.Run("SetupDataset", func(t *testing.T) {
		datasetProps := map[string]interface{}{
			"datasetId": datasetID,
			"location":  testutil.Region,
		}

		propsJSON, err := json.Marshal(datasetProps)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: DatasetResourceType,
			Properties:   propsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := datasetProv.Create(ctx, createReq)
		require.NoError(t, err)
		require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)
		t.Logf("Test dataset created: %s", createResult.ProgressResult.NativeID)
	})

	// Test 1: Create SQL UDF
	t.Run("CreateUDF", func(t *testing.T) {
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

		nativeID = createResult.ProgressResult.NativeID
		t.Logf("UDF created successfully with native ID: %s", nativeID)

		// Verify native ID format
		expectedPrefix := fmt.Sprintf("projects/%s/datasets/%s/routines/%s", testutil.Project, datasetID, routineID)
		assert.Equal(t, expectedPrefix, nativeID)
	})

	// Test 2: Read Routine
	t.Run("ReadRoutine", func(t *testing.T) {
		require.NotEmpty(t, nativeID)

		readReq := &resource.ReadRequest{
			NativeID: nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		readResult, err := routineProv.Read(ctx, readReq)
		require.NoError(t, err)
		require.NotNil(t, readResult)
		require.NotEmpty(t, readResult.Properties)

		t.Logf("Read routine successfully")

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
	})

	// Test 3: List Routines
	t.Run("ListRoutines", func(t *testing.T) {
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
				t.Logf("Found our routine in list: %s", id)
				break
			}
		}
		assert.True(t, found, "Our routine should be in the list")
	})

	// Test 4: Update Routine (should return NotUpdatable)
	t.Run("UpdateRoutine", func(t *testing.T) {
		require.NotEmpty(t, nativeID)

		updatedProperties := map[string]interface{}{
			"datasetId":   datasetID,
			"routineId":   routineID,
			"description": "Updated description",
		}

		propsJSON, err := json.Marshal(updatedProperties)
		require.NoError(t, err)

		updateReq := &resource.UpdateRequest{
			NativeID:          nativeID,
			ResourceType:      RoutineResourceType,
			DesiredProperties: propsJSON,
			TargetConfig:      testutil.TargetConfig,
		}

		updateResult, err := routineProv.Update(ctx, updateReq)
		require.NoError(t, err)
		require.NotNil(t, updateResult)
		require.Equal(t, resource.OperationStatusFailure, updateResult.ProgressResult.OperationStatus)
		require.Equal(t, resource.OperationErrorCodeNotUpdatable, updateResult.ProgressResult.ErrorCode)

		t.Logf("Update correctly returned NotUpdatable status")
	})

	// Test 5: Delete Routine
	t.Run("DeleteRoutine", func(t *testing.T) {
		require.NotEmpty(t, nativeID)

		deleteReq := &resource.DeleteRequest{
			NativeID: nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		deleteResult, err := routineProv.Delete(ctx, deleteReq)
		require.NoError(t, err)
		require.NotNil(t, deleteResult)
		require.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus)

		t.Logf("Routine deleted successfully: %s", nativeID)
	})

	// Test 6: Verify Routine is Deleted
	t.Run("VerifyRoutineDeleted", func(t *testing.T) {
		require.NotEmpty(t, nativeID)

		readReq := &resource.ReadRequest{
			NativeID: nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		readResult, err := routineProv.Read(ctx, readReq)
		require.NoError(t, err)
		require.NotNil(t, readResult)
		require.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode)

		t.Logf("Verified routine is deleted (not found): %s", nativeID)
	})

	// Cleanup: Delete dataset
	t.Run("CleanupDataset", func(t *testing.T) {
		datasetNativeID := fmt.Sprintf("projects/%s/datasets/%s", testutil.Project, datasetID)
		deleteReq := &resource.DeleteRequest{
			NativeID: datasetNativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, err := datasetProv.Delete(ctx, deleteReq)
		require.NoError(t, err)
		t.Logf("Test dataset cleaned up")
	})
}

// TestRoutineCreateJavaScriptUDF tests creating a JavaScript UDF
func TestRoutineCreateJavaScriptUDF(t *testing.T) {
	datasetProv := &Dataset{cfg: testutil.Config}
	routineProv := &Routine{cfg: testutil.Config}

	datasetID := fmt.Sprintf("formae_js_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))
	routineID := fmt.Sprintf("js_udf_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))
	ctx := context.Background()

	// Setup: Create dataset
	datasetProps := map[string]interface{}{
		"datasetId": datasetID,
		"location":  testutil.Region,
	}
	propsJSON, _ := json.Marshal(datasetProps)
	createReq := &resource.CreateRequest{
		ResourceType: DatasetResourceType,
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}
	_, err := datasetProv.Create(ctx, createReq)
	require.NoError(t, err)

	t.Run("CreateJavaScriptUDF", func(t *testing.T) {
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
		t.Logf("JavaScript UDF created: %s", nativeID)

		// Read and verify
		readReq := &resource.ReadRequest{
			NativeID: nativeID,
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

		// Cleanup
		deleteReq := &resource.DeleteRequest{
			NativeID: nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, err = routineProv.Delete(ctx, deleteReq)
		require.NoError(t, err)
	})

	// Cleanup dataset
	datasetNativeID := fmt.Sprintf("projects/%s/datasets/%s", testutil.Project, datasetID)
	deleteReq := &resource.DeleteRequest{
		NativeID: datasetNativeID,
		TargetConfig: testutil.TargetConfig,
	}
	_, _ = datasetProv.Delete(ctx, deleteReq)
}

// TestRoutineCreateStoredProcedure tests creating a stored procedure
func TestRoutineCreateStoredProcedure(t *testing.T) {
	datasetProv := &Dataset{cfg: testutil.Config}
	routineProv := &Routine{cfg: testutil.Config}

	datasetID := fmt.Sprintf("formae_proc_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))
	routineID := fmt.Sprintf("test_proc_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))
	ctx := context.Background()

	// Setup: Create dataset
	datasetProps := map[string]interface{}{
		"datasetId": datasetID,
		"location":  testutil.Region,
	}
	propsJSON, _ := json.Marshal(datasetProps)
	createReq := &resource.CreateRequest{
		ResourceType: DatasetResourceType,
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}
	_, err := datasetProv.Create(ctx, createReq)
	require.NoError(t, err)

	t.Run("CreateStoredProcedure", func(t *testing.T) {
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
		t.Logf("Stored procedure created: %s", nativeID)

		// Read and verify
		readReq := &resource.ReadRequest{
			NativeID: nativeID,
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

		// Cleanup
		deleteReq := &resource.DeleteRequest{
			NativeID: nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, err = routineProv.Delete(ctx, deleteReq)
		require.NoError(t, err)
	})

	// Cleanup dataset
	datasetNativeID := fmt.Sprintf("projects/%s/datasets/%s", testutil.Project, datasetID)
	deleteReq := &resource.DeleteRequest{
		NativeID: datasetNativeID,
		TargetConfig: testutil.TargetConfig,
	}
	_, _ = datasetProv.Delete(ctx, deleteReq)
}

// TestRoutineCreateTableValuedFunction tests creating a table-valued function
func TestRoutineCreateTableValuedFunction(t *testing.T) {
	datasetProv := &Dataset{cfg: testutil.Config}
	routineProv := &Routine{cfg: testutil.Config}

	datasetID := fmt.Sprintf("formae_tvf_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))
	routineID := fmt.Sprintf("tvf_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))
	ctx := context.Background()

	// Setup: Create dataset
	datasetProps := map[string]interface{}{
		"datasetId": datasetID,
		"location":  testutil.Region,
	}
	propsJSON, _ := json.Marshal(datasetProps)
	createReq := &resource.CreateRequest{
		ResourceType: DatasetResourceType,
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}
	_, err := datasetProv.Create(ctx, createReq)
	require.NoError(t, err)

	t.Run("CreateTableValuedFunction", func(t *testing.T) {
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
		t.Logf("Table-valued function created: %s", nativeID)

		// Read and verify
		readReq := &resource.ReadRequest{
			NativeID: nativeID,
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

		// Cleanup
		deleteReq := &resource.DeleteRequest{
			NativeID: nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, err = routineProv.Delete(ctx, deleteReq)
		require.NoError(t, err)
	})

	// Cleanup dataset
	datasetNativeID := fmt.Sprintf("projects/%s/datasets/%s", testutil.Project, datasetID)
	deleteReq := &resource.DeleteRequest{
		NativeID: datasetNativeID,
		TargetConfig: testutil.TargetConfig,
	}
	_, _ = datasetProv.Delete(ctx, deleteReq)
}

// TestRoutineInvalidCreate tests error handling
func TestRoutineInvalidCreate(t *testing.T) {
	routineProv := &Routine{cfg: testutil.Config}
	ctx := context.Background()

	t.Run("CreateWithMissingDatasetID", func(t *testing.T) {
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
	})

	t.Run("CreateWithMissingRoutineID", func(t *testing.T) {
		invalidProperties := map[string]interface{}{
			"datasetId": "test_dataset",
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

		t.Logf("Correctly rejected create with missing routineId")
	})
}

// TestRoutineListWithoutDatasetID tests that listing without datasetId fails properly
func TestRoutineListWithoutDatasetID(t *testing.T) {
	routineProv := &Routine{cfg: testutil.Config}
	ctx := context.Background()

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
