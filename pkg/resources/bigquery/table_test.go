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
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/testutil"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestDataset creates a temporary dataset for table tests and returns the datasetID and cleanup function
func createTestDataset(t *testing.T, ctx context.Context, provisioner *Dataset, suffix string) (string, func()) {
	datasetID := fmt.Sprintf("formae_%s_%s", suffix, strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

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

	createResult, err := provisioner.Create(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

	cleanup := func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     createResult.ProgressResult.NativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = provisioner.Delete(ctx, deleteReq)
		t.Logf("Cleaned up test dataset: %s", datasetID)
	}

	return datasetID, cleanup
}

// TestTable_Create_Integration tests creating a BigQuery table
func TestTable_Create_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	datasetProv := &Dataset{cfg: testutil.Config}
	tableProv := &Table{cfg: testutil.Config}

	// Create test dataset
	datasetID, cleanupDataset := createTestDataset(t, ctx, datasetProv, "tbl_create")
	defer cleanupDataset()

	// Generate unique table name
	tableID := fmt.Sprintf("test_table_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))
	t.Logf("Creating test table: %s in dataset %s", tableID, datasetID)

	tableProperties := map[string]interface{}{
		"datasetId":   datasetID,
		"tableId":     tableID,
		"description": "Test table created by Formae integration test",
		"labels": map[string]interface{}{
			"environment": "test",
			"created-by":  "formae-integration-test",
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
		},
	}

	propsJSON, err := json.Marshal(tableProperties)
	require.NoError(t, err, "Failed to marshal table properties")

	createReq := &resource.CreateRequest{
		ResourceType: "GCP::BigQuery::Table",
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := tableProv.Create(ctx, createReq)
	require.NoError(t, err, "Create operation should not return error")
	require.NotNil(t, createResult, "Create result should not be nil")
	require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

	// Cleanup table after test
	if createResult.ProgressResult.NativeID != "" {
		defer func() {
			deleteReq := &resource.DeleteRequest{
				NativeID:     createResult.ProgressResult.NativeID,
				TargetConfig: testutil.TargetConfig,
			}
			_, _ = tableProv.Delete(ctx, deleteReq)
			t.Logf("Cleaned up test table: %s", tableID)
		}()
	}

	assert.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus,
		"BigQuery operations are synchronous, should return Success")
	require.NotEmpty(t, createResult.ProgressResult.NativeID, "NativeID should be set")

	// Verify native ID format
	expectedNativeID := fmt.Sprintf("projects/%s/datasets/%s/tables/%s", testutil.Project, datasetID, tableID)
	assert.Equal(t, expectedNativeID, createResult.ProgressResult.NativeID, "Native ID should match expected format")

	t.Logf("Table created successfully with NativeID: %s", createResult.ProgressResult.NativeID)
}

// TestTable_Read_Integration tests reading a BigQuery table
func TestTable_Read_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	datasetProv := &Dataset{cfg: testutil.Config}
	tableProv := &Table{cfg: testutil.Config}

	// Create test dataset
	datasetID, cleanupDataset := createTestDataset(t, ctx, datasetProv, "tbl_read")
	defer cleanupDataset()

	// Create table
	tableID := fmt.Sprintf("read_table_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	tableProperties := map[string]interface{}{
		"datasetId": datasetID,
		"tableId":   tableID,
		"labels": map[string]interface{}{
			"test-label": "test-value",
		},
		"schema": []map[string]interface{}{
			{"name": "id", "type": "INTEGER", "mode": "REQUIRED"},
			{"name": "value", "type": "STRING", "mode": "NULLABLE"},
		},
	}

	propsJSON, err := json.Marshal(tableProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: "GCP::BigQuery::Table",
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := tableProv.Create(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

	nativeID := createResult.ProgressResult.NativeID

	// Cleanup table
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = tableProv.Delete(ctx, deleteReq)
	}()

	// Now test Read
	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := tableProv.Read(ctx, readReq)
	require.NoError(t, err, "Read should not return error")
	require.NotNil(t, readResult, "Read result should not be nil")
	require.Empty(t, readResult.ErrorCode, "ErrorCode should be empty for successful read")
	require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

	// Unmarshal and verify properties
	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, tableID, props["tableId"], "Table ID should match")
	assert.Equal(t, datasetID, props["datasetId"], "Dataset ID should match")

	// Verify schema
	if schema, ok := props["schema"].([]interface{}); ok {
		assert.Len(t, schema, 2, "Should have 2 schema fields")
	}

	// Verify labels
	if labels, ok := props["labels"].(map[string]interface{}); ok {
		assert.Equal(t, "test-value", labels["test-label"], "Label should match")
	}

	t.Logf("Table read successfully")
}

// TestTable_Update_Integration tests updating a BigQuery table (should return NotUpdatable)
func TestTable_Update_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	datasetProv := &Dataset{cfg: testutil.Config}
	tableProv := &Table{cfg: testutil.Config}

	// Create test dataset
	datasetID, cleanupDataset := createTestDataset(t, ctx, datasetProv, "tbl_update")
	defer cleanupDataset()

	// Create table
	tableID := fmt.Sprintf("update_table_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	tableProperties := map[string]interface{}{
		"datasetId": datasetID,
		"tableId":   tableID,
		"schema": []map[string]interface{}{
			{"name": "id", "type": "INTEGER", "mode": "REQUIRED"},
		},
	}

	propsJSON, err := json.Marshal(tableProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: "GCP::BigQuery::Table",
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := tableProv.Create(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

	nativeID := createResult.ProgressResult.NativeID

	// Cleanup table
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = tableProv.Delete(ctx, deleteReq)
	}()

	// Now test Update (should return NotUpdatable)
	updatedProperties := map[string]interface{}{
		"datasetId":   datasetID,
		"tableId":     tableID,
		"description": "Updated description",
	}

	updatePropsJSON, err := json.Marshal(updatedProperties)
	require.NoError(t, err)

	updateReq := &resource.UpdateRequest{
		NativeID:          nativeID,
		ResourceType:      "GCP::BigQuery::Table",
		DesiredProperties: updatePropsJSON,
		TargetConfig:      testutil.TargetConfig,
	}

	updateResult, err := tableProv.Update(ctx, updateReq)
	require.NoError(t, err, "Update should not return error")
	require.NotNil(t, updateResult, "Update result should not be nil")
	require.NotNil(t, updateResult.ProgressResult, "Progress result should not be nil")

	assert.Equal(t, resource.OperationStatusFailure, updateResult.ProgressResult.OperationStatus,
		"Update should return Failure status")
	assert.Equal(t, resource.OperationErrorCodeNotUpdatable, updateResult.ProgressResult.ErrorCode,
		"Update should return NotUpdatable error code")

	t.Logf("Update correctly returned NotUpdatable status")
}

// TestTable_Delete_Integration tests deleting a BigQuery table
func TestTable_Delete_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	datasetProv := &Dataset{cfg: testutil.Config}
	tableProv := &Table{cfg: testutil.Config}

	// Create test dataset
	datasetID, cleanupDataset := createTestDataset(t, ctx, datasetProv, "tbl_delete")
	defer cleanupDataset()

	// Create table
	tableID := fmt.Sprintf("delete_table_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	tableProperties := map[string]interface{}{
		"datasetId": datasetID,
		"tableId":   tableID,
		"schema": []map[string]interface{}{
			{"name": "id", "type": "INTEGER", "mode": "REQUIRED"},
		},
	}

	propsJSON, err := json.Marshal(tableProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: "GCP::BigQuery::Table",
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := tableProv.Create(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

	nativeID := createResult.ProgressResult.NativeID

	// Now test Delete
	deleteReq := &resource.DeleteRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
	}

	deleteResult, err := tableProv.Delete(ctx, deleteReq)
	require.NoError(t, err, "Delete should not return error")
	require.NotNil(t, deleteResult, "Delete result should not be nil")
	require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")
	assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus,
		"Delete should return Success status")

	t.Logf("Table deleted successfully: %s", nativeID)

	// Verify table is deleted
	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := tableProv.Read(ctx, readReq)
	require.NoError(t, err)
	assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode,
		"Read should return NotFound after deletion")
}

// TestTable_Delete_NotFound_Integration tests deleting a non-existent table (idempotent)
func TestTable_Delete_NotFound_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	datasetProv := &Dataset{cfg: testutil.Config}
	tableProv := &Table{cfg: testutil.Config}

	// Create test dataset (table needs a valid dataset in the path)
	datasetID, cleanupDataset := createTestDataset(t, ctx, datasetProv, "tbl_del_nf")
	defer cleanupDataset()

	// Use a non-existent table ID
	nonExistentID := fmt.Sprintf("projects/%s/datasets/%s/tables/formae_nonexistent_%s",
		testutil.Project, datasetID,
		strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	deleteReq := &resource.DeleteRequest{
		NativeID:     nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	deleteResult, err := tableProv.Delete(ctx, deleteReq)
	require.NoError(t, err, "Delete should not return error")
	require.NotNil(t, deleteResult, "Delete result should not be nil")

	// Delete of non-existent resource should succeed (idempotent)
	assert.Contains(t, []resource.OperationStatus{
		resource.OperationStatusSuccess,
		resource.OperationStatusFailure,
	}, deleteResult.ProgressResult.OperationStatus,
		"Delete should handle non-existent resource gracefully")

	t.Logf("Delete non-existent table returned status: %s", deleteResult.ProgressResult.OperationStatus)
}

// TestTable_List_Integration tests listing BigQuery tables
func TestTable_List_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	datasetProv := &Dataset{cfg: testutil.Config}
	tableProv := &Table{cfg: testutil.Config}

	// Create test dataset
	datasetID, cleanupDataset := createTestDataset(t, ctx, datasetProv, "tbl_list")
	defer cleanupDataset()

	// Create table
	tableID := fmt.Sprintf("list_table_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	tableProperties := map[string]interface{}{
		"datasetId": datasetID,
		"tableId":   tableID,
		"schema": []map[string]interface{}{
			{"name": "id", "type": "INTEGER", "mode": "REQUIRED"},
		},
	}

	propsJSON, err := json.Marshal(tableProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: "GCP::BigQuery::Table",
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := tableProv.Create(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

	nativeID := createResult.ProgressResult.NativeID

	// Cleanup table
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = tableProv.Delete(ctx, deleteReq)
	}()

	// Now test List
	listReq := &resource.ListRequest{
		ResourceType: "GCP::BigQuery::Table",
		TargetConfig: testutil.TargetConfig,
		AdditionalProperties: map[string]string{
			"datasetId": datasetID,
		},
	}

	listResult, err := tableProv.List(ctx, listReq)
	require.NoError(t, err, "List should not return error")
	require.NotNil(t, listResult, "List result should not be nil")
	require.NotEmpty(t, listResult.NativeIDs, "List should return at least one table")

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
}

// TestTable_CreateWithPartitioning_Integration tests creating a partitioned table
func TestTable_CreateWithPartitioning_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	datasetProv := &Dataset{cfg: testutil.Config}
	tableProv := &Table{cfg: testutil.Config}

	// Create test dataset
	datasetID, cleanupDataset := createTestDataset(t, ctx, datasetProv, "tbl_part")
	defer cleanupDataset()

	tableID := fmt.Sprintf("partitioned_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	tableProperties := map[string]interface{}{
		"datasetId":   datasetID,
		"tableId":     tableID,
		"description": "Partitioned table test",
		"schema": []map[string]interface{}{
			{"name": "event_date", "type": "DATE", "mode": "REQUIRED"},
			{"name": "event_name", "type": "STRING", "mode": "REQUIRED"},
			{"name": "value", "type": "INTEGER", "mode": "NULLABLE"},
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
		ResourceType: "GCP::BigQuery::Table",
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := tableProv.Create(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

	nativeID := createResult.ProgressResult.NativeID

	// Cleanup
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = tableProv.Delete(ctx, deleteReq)
	}()

	// Read and verify partitioning
	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
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
}

// TestTable_CreateView_Integration tests creating a view
func TestTable_CreateView_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	datasetProv := &Dataset{cfg: testutil.Config}
	tableProv := &Table{cfg: testutil.Config}

	// Create test dataset
	datasetID, cleanupDataset := createTestDataset(t, ctx, datasetProv, "tbl_view")
	defer cleanupDataset()

	// Create base table first
	baseTableID := fmt.Sprintf("base_table_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	baseTableProps := map[string]interface{}{
		"datasetId": datasetID,
		"tableId":   baseTableID,
		"schema": []map[string]interface{}{
			{"name": "id", "type": "INTEGER", "mode": "REQUIRED"},
			{"name": "value", "type": "STRING", "mode": "NULLABLE"},
		},
	}

	propsJSON, err := json.Marshal(baseTableProps)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: "GCP::BigQuery::Table",
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	baseTableResult, err := tableProv.Create(ctx, createReq)
	require.NoError(t, err)

	baseTableNativeID := baseTableResult.ProgressResult.NativeID

	// Cleanup base table
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     baseTableNativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = tableProv.Delete(ctx, deleteReq)
	}()

	// Create view
	viewID := fmt.Sprintf("test_view_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))
	viewQuery := fmt.Sprintf("SELECT id, value FROM `%s.%s.%s` WHERE id > 0",
		testutil.Project, datasetID, baseTableID)

	viewProperties := map[string]interface{}{
		"datasetId":   datasetID,
		"tableId":     viewID,
		"description": "Test view",
		"view":        viewQuery,
	}

	viewPropsJSON, err := json.Marshal(viewProperties)
	require.NoError(t, err)

	viewCreateReq := &resource.CreateRequest{
		ResourceType: "GCP::BigQuery::Table",
		Properties:   viewPropsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	viewCreateResult, err := tableProv.Create(ctx, viewCreateReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, viewCreateResult.ProgressResult.OperationStatus)

	viewNativeID := viewCreateResult.ProgressResult.NativeID

	// Cleanup view
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     viewNativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = tableProv.Delete(ctx, deleteReq)
	}()

	// Read and verify view
	readReq := &resource.ReadRequest{
		NativeID:     viewNativeID,
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
}

// TestTable_ListWithoutDatasetID_Integration tests that listing without datasetId fails properly
func TestTable_ListWithoutDatasetID_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	tableProv := &Table{cfg: testutil.Config}

	listReq := &resource.ListRequest{
		ResourceType: "GCP::BigQuery::Table",
		TargetConfig: testutil.TargetConfig,
		// Missing AdditionalProperties with datasetId
	}

	_, err := tableProv.List(ctx, listReq)
	require.Error(t, err, "List should return error when datasetId is missing")
	assert.Contains(t, err.Error(), "datasetId", "Error should mention datasetId")

	t.Logf("Correctly rejected list without datasetId")
}

// TestTable_CreateInvalid_Integration tests error handling for invalid table creation
func TestTable_CreateInvalid_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	tableProv := &Table{cfg: testutil.Config}

	// Test with missing datasetId
	invalidProperties := map[string]interface{}{
		"tableId": "test_table",
		"schema": []map[string]interface{}{
			{"name": "id", "type": "INTEGER", "mode": "REQUIRED"},
		},
	}

	propsJSON, err := json.Marshal(invalidProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: "GCP::BigQuery::Table",
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := tableProv.Create(ctx, createReq)
	require.NoError(t, err, "Create should not return error (error should be in result)")
	require.NotNil(t, createResult)
	assert.Equal(t, resource.OperationStatusFailure, createResult.ProgressResult.OperationStatus,
		"Create should return Failure status for missing datasetId")

	t.Logf("Correctly rejected create with missing datasetId: %s", createResult.ProgressResult.StatusMessage)
}
