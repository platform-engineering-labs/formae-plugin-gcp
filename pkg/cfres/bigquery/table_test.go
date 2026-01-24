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
	TableResourceType = "GCP::BigQuery::Table"
)

// TestTableCreate tests the full CRUD lifecycle of a BigQuery table
func TestTableCreate(t *testing.T) {
	// Create provisioners
	datasetProv := &Dataset{cfg: testutil.Config}
	tableProv := &Table{cfg: testutil.Config}

	// Create a test dataset first
	datasetID := fmt.Sprintf("formae_table_test_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))
	tableID := fmt.Sprintf("test_table_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))
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

	// Test 1: Create Table
	t.Run("CreateTable", func(t *testing.T) {
		tableProperties := map[string]interface{}{
			"datasetId":   datasetID,
			"tableId":     tableID,
			"description": "Test table created by Formae integration tests",
			"labels": map[string]interface{}{
				"test":        "formae-bigquery-table",
				"environment": "integration-test",
			},
			"schema": []map[string]interface{}{
				{
					"name":        "id",
					"type":        "INTEGER",
					"mode":        "REQUIRED",
					"description": "Unique identifier",
				},
				{
					"name":        "name",
					"type":        "STRING",
					"mode":        "REQUIRED",
					"description": "Name field",
				},
				{
					"name":        "timestamp",
					"type":        "TIMESTAMP",
					"mode":        "NULLABLE",
					"description": "Timestamp field",
				},
				{
					"name":        "metadata",
					"type":        "RECORD",
					"mode":        "NULLABLE",
					"description": "Nested record",
					"fields": []map[string]interface{}{
						{
							"name": "key",
							"type": "STRING",
							"mode": "NULLABLE",
						},
						{
							"name": "value",
							"type": "STRING",
							"mode": "NULLABLE",
						},
					},
				},
			},
		}

		propsJSON, err := json.Marshal(tableProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: TableResourceType,
			Properties:   propsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := tableProv.Create(ctx, createReq)
		require.NoError(t, err)
		require.NotNil(t, createResult)
		require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)
		require.NotEmpty(t, createResult.ProgressResult.NativeID)

		nativeID = createResult.ProgressResult.NativeID
		t.Logf("Table created successfully with native ID: %s", nativeID)

		// Verify native ID format
		expectedPrefix := fmt.Sprintf("projects/%s/datasets/%s/tables/%s", testutil.Project, datasetID, tableID)
		assert.Equal(t, expectedPrefix, nativeID)
	})

	// Test 2: Read Table
	t.Run("ReadTable", func(t *testing.T) {
		require.NotEmpty(t, nativeID)

		readReq := &resource.ReadRequest{
			NativeID: nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		readResult, err := tableProv.Read(ctx, readReq)
		require.NoError(t, err)
		require.NotNil(t, readResult)
		require.NotEmpty(t, readResult.Properties)

		t.Logf("Read table successfully")

		// Unmarshal and verify properties
		var props map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &props)
		require.NoError(t, err)

		assert.Equal(t, tableID, props["tableId"])
		assert.Equal(t, datasetID, props["datasetId"])

		// Verify schema
		if schema, ok := props["schema"].([]interface{}); ok {
			assert.Len(t, schema, 4, "Should have 4 schema fields")
			t.Logf("Schema has %d fields", len(schema))
		}

		// Verify labels
		if labels, ok := props["labels"].(map[string]interface{}); ok {
			assert.Equal(t, "formae-bigquery-table", labels["test"])
		}
	})

	// Test 3: List Tables
	t.Run("ListTables", func(t *testing.T) {
		listReq := &resource.ListRequest{
			ResourceType: TableResourceType,
			TargetConfig: testutil.TargetConfig,
			AdditionalProperties: map[string]string{
				"datasetId": datasetID,
			},
		}

		listResult, err := tableProv.List(ctx, listReq)
		require.NoError(t, err)
		require.NotNil(t, listResult)
		require.NotEmpty(t, listResult.NativeIDs)

		t.Logf("Listed %d tables", len(listResult.NativeIDs))

		// Verify our table is in the list
		found := false
		for _, id := range listResult.NativeIDs {
			if id == nativeID {
				found = true
				t.Logf("Found our table in list: %s", id)
				break
			}
		}
		assert.True(t, found, "Our table should be in the list")
	})

	// Test 4: Update Table (should return NotUpdatable)
	t.Run("UpdateTable", func(t *testing.T) {
		require.NotEmpty(t, nativeID)

		updatedProperties := map[string]interface{}{
			"datasetId":   datasetID,
			"tableId":     tableID,
			"description": "Updated description",
		}

		propsJSON, err := json.Marshal(updatedProperties)
		require.NoError(t, err)

		updateReq := &resource.UpdateRequest{
			NativeID:          nativeID,
			ResourceType:      TableResourceType,
			DesiredProperties: propsJSON,
			TargetConfig:      testutil.TargetConfig,
		}

		updateResult, err := tableProv.Update(ctx, updateReq)
		require.NoError(t, err)
		require.NotNil(t, updateResult)
		require.Equal(t, resource.OperationStatusFailure, updateResult.ProgressResult.OperationStatus)
		require.Equal(t, resource.OperationErrorCodeNotUpdatable, updateResult.ProgressResult.ErrorCode)

		t.Logf("Update correctly returned NotUpdatable status")
	})

	// Test 5: Delete Table
	t.Run("DeleteTable", func(t *testing.T) {
		require.NotEmpty(t, nativeID)

		deleteReq := &resource.DeleteRequest{
			NativeID: nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		deleteResult, err := tableProv.Delete(ctx, deleteReq)
		require.NoError(t, err)
		require.NotNil(t, deleteResult)
		require.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus)

		t.Logf("Table deleted successfully: %s", nativeID)
	})

	// Test 6: Verify Table is Deleted
	t.Run("VerifyTableDeleted", func(t *testing.T) {
		require.NotEmpty(t, nativeID)

		readReq := &resource.ReadRequest{
			NativeID: nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		readResult, err := tableProv.Read(ctx, readReq)
		require.NoError(t, err)
		require.NotNil(t, readResult)
		require.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode)

		t.Logf("Verified table is deleted (not found): %s", nativeID)
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

// TestTableCreateWithPartitioning tests creating a partitioned table
func TestTableCreateWithPartitioning(t *testing.T) {
	datasetProv := &Dataset{cfg: testutil.Config}
	tableProv := &Table{cfg: testutil.Config}

	datasetID := fmt.Sprintf("formae_part_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))
	tableID := fmt.Sprintf("partitioned_table_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))
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

	t.Run("CreatePartitionedTable", func(t *testing.T) {
		tableProperties := map[string]interface{}{
			"datasetId":   datasetID,
			"tableId":     tableID,
			"description": "Partitioned table test",
			"schema": []map[string]interface{}{
				{
					"name": "event_date",
					"type": "DATE",
					"mode": "REQUIRED",
				},
				{
					"name": "event_name",
					"type": "STRING",
					"mode": "REQUIRED",
				},
				{
					"name": "value",
					"type": "INTEGER",
					"mode": "NULLABLE",
				},
			},
			"timePartitioning": map[string]interface{}{
				"type":  "DAY",
				"field": "event_date",
			},
			"clustering": []string{"event_name"},
		}

		propsJSON, err := json.Marshal(tableProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: TableResourceType,
			Properties:   propsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := tableProv.Create(ctx, createReq)
		require.NoError(t, err)
		require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

		nativeID := createResult.ProgressResult.NativeID
		t.Logf("Partitioned table created: %s", nativeID)

		// Read and verify partitioning
		readReq := &resource.ReadRequest{
			NativeID: nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		readResult, err := tableProv.Read(ctx, readReq)
		require.NoError(t, err)

		var props map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &props)
		require.NoError(t, err)

		// Verify time partitioning
		if timePartitioning, ok := props["timePartitioning"].(map[string]interface{}); ok {
			assert.Equal(t, "DAY", timePartitioning["type"])
			assert.Equal(t, "event_date", timePartitioning["field"])
			t.Logf("Time partitioning verified")
		}

		// Verify clustering
		if clustering, ok := props["clustering"].([]interface{}); ok {
			assert.Len(t, clustering, 1)
			assert.Equal(t, "event_name", clustering[0])
			t.Logf("Clustering verified")
		}

		// Cleanup
		deleteReq := &resource.DeleteRequest{
			NativeID: nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, err = tableProv.Delete(ctx, deleteReq)
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

// TestTableCreateView tests creating a view
func TestTableCreateView(t *testing.T) {
	datasetProv := &Dataset{cfg: testutil.Config}
	tableProv := &Table{cfg: testutil.Config}

	datasetID := fmt.Sprintf("formae_view_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))
	baseTableID := fmt.Sprintf("base_table_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))
	viewID := fmt.Sprintf("test_view_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))
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

	// Create base table first
	baseTableProps := map[string]interface{}{
		"datasetId": datasetID,
		"tableId":   baseTableID,
		"schema": []map[string]interface{}{
			{
				"name": "id",
				"type": "INTEGER",
				"mode": "REQUIRED",
			},
			{
				"name": "value",
				"type": "STRING",
				"mode": "NULLABLE",
			},
		},
	}
	propsJSON, _ = json.Marshal(baseTableProps)
	createReq = &resource.CreateRequest{
		ResourceType: TableResourceType,
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}
	baseTableResult, err := tableProv.Create(ctx, createReq)
	require.NoError(t, err)
	baseTableNativeID := baseTableResult.ProgressResult.NativeID

	t.Run("CreateView", func(t *testing.T) {
		viewQuery := fmt.Sprintf("SELECT id, value FROM `%s.%s.%s` WHERE id > 0",
			testutil.Project, datasetID, baseTableID)

		viewProperties := map[string]interface{}{
			"datasetId":   datasetID,
			"tableId":     viewID,
			"description": "Test view",
			"view":        viewQuery,
		}

		propsJSON, err := json.Marshal(viewProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: TableResourceType,
			Properties:   propsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := tableProv.Create(ctx, createReq)
		require.NoError(t, err)
		require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

		viewNativeID := createResult.ProgressResult.NativeID
		t.Logf("View created: %s", viewNativeID)

		// Read and verify view query
		readReq := &resource.ReadRequest{
			NativeID: viewNativeID,
			TargetConfig: testutil.TargetConfig,
		}

		readResult, err := tableProv.Read(ctx, readReq)
		require.NoError(t, err)

		var props map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &props)
		require.NoError(t, err)

		assert.Contains(t, props["view"], "SELECT")
		assert.Equal(t, "VIEW", props["type"])
		t.Logf("View query verified")

		// Cleanup
		deleteReq := &resource.DeleteRequest{
			NativeID: viewNativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, err = tableProv.Delete(ctx, deleteReq)
		require.NoError(t, err)
	})

	// Cleanup base table and dataset
	deleteReq := &resource.DeleteRequest{
		NativeID: baseTableNativeID,
		TargetConfig: testutil.TargetConfig,
	}
	_, _ = tableProv.Delete(ctx, deleteReq)

	datasetNativeID := fmt.Sprintf("projects/%s/datasets/%s", testutil.Project, datasetID)
	deleteReq = &resource.DeleteRequest{
		NativeID: datasetNativeID,
		TargetConfig: testutil.TargetConfig,
	}
	_, _ = datasetProv.Delete(ctx, deleteReq)
}

// TestTableInvalidCreate tests error handling
func TestTableInvalidCreate(t *testing.T) {
	tableProv := &Table{cfg: testutil.Config}
	ctx := context.Background()

	t.Run("CreateWithMissingDatasetID", func(t *testing.T) {
		invalidProperties := map[string]interface{}{
			"tableId": "test_table",
			"schema": []map[string]interface{}{
				{
					"name": "id",
					"type": "INTEGER",
					"mode": "REQUIRED",
				},
			},
		}

		propsJSON, err := json.Marshal(invalidProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: TableResourceType,
			Properties:   propsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := tableProv.Create(ctx, createReq)
		require.NoError(t, err)
		require.Equal(t, resource.OperationStatusFailure, createResult.ProgressResult.OperationStatus)
		assert.Contains(t, createResult.ProgressResult.StatusMessage, "required")

		t.Logf("Correctly rejected create with missing datasetId")
	})

	t.Run("CreateWithMissingTableID", func(t *testing.T) {
		invalidProperties := map[string]interface{}{
			"datasetId": "test_dataset",
			"schema": []map[string]interface{}{
				{
					"name": "id",
					"type": "INTEGER",
					"mode": "REQUIRED",
				},
			},
		}

		propsJSON, err := json.Marshal(invalidProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: TableResourceType,
			Properties:   propsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := tableProv.Create(ctx, createReq)
		require.NoError(t, err)
		require.Equal(t, resource.OperationStatusFailure, createResult.ProgressResult.OperationStatus)
		assert.Contains(t, createResult.ProgressResult.StatusMessage, "required")

		t.Logf("Correctly rejected create with missing tableId")
	})
}

// TestTableListWithoutDatasetID tests that listing without datasetId fails properly
func TestTableListWithoutDatasetID(t *testing.T) {
	tableProv := &Table{cfg: testutil.Config}
	ctx := context.Background()

	listReq := &resource.ListRequest{
		ResourceType: TableResourceType,
		TargetConfig: testutil.TargetConfig,
		// Missing AdditionalProperties with datasetId
	}

	_, err := tableProv.List(ctx, listReq)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "datasetId")

	t.Logf("Correctly rejected list without datasetId")
}
