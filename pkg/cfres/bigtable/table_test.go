// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration

package bigtable

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/testutil"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTable_Create_Integration tests creating a Bigtable table.
func TestTable_Create_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	instanceName, _, cleanupInstance := createTestInstance(t, ctx)
	defer cleanupInstance()

	tableProv, err := NewBigtableProvisioner(testutil.Config, TableResourceType)
	require.NoError(t, err)

	tableName := fmt.Sprintf("table_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	tableProperties := map[string]interface{}{
		"instance": instanceName,
		"name":     tableName,
		"columnFamilies": map[string]interface{}{
			"cf1": map[string]interface{}{
				"gcRule": map[string]interface{}{
					"maxNumVersions": 3,
				},
			},
			"cf2": map[string]interface{}{
				"gcRule": map[string]interface{}{
					"maxAge": "259200s",
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
	require.NotNil(t, createResult.ProgressResult)

	var tableNativeID string
	if createResult.ProgressResult.OperationStatus == resource.OperationStatusSuccess {
		tableNativeID = createResult.ProgressResult.NativeID
	} else if createResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
		statusResult, err := testutil.PollUntilComplete(t, ctx, tableProv,
			createResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
				MaxAttempts:   30,
				CheckInterval: 5 * time.Second,
				ResourceType:  TableResourceType,
				OperationName: "Create",
			})
		require.NoError(t, err)
		tableNativeID = statusResult.ProgressResult.NativeID
	} else {
		t.Fatalf("Unexpected operation status: %v (error: %s, message: %s)",
			createResult.ProgressResult.OperationStatus,
			createResult.ProgressResult.ErrorCode,
			createResult.ProgressResult.StatusMessage)
	}

	// Cleanup table
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     tableNativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = tableProv.Delete(ctx, deleteReq)
	}()

	require.NotEmpty(t, tableNativeID)
	expectedID := fmt.Sprintf("projects/%s/instances/%s/tables/%s", testutil.Project, instanceName, tableName)
	assert.Equal(t, expectedID, tableNativeID)
	t.Logf("Table created: %s", tableNativeID)
}

// TestTable_Read_Integration tests reading a Bigtable table.
func TestTable_Read_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	instanceName, _, cleanupInstance := createTestInstance(t, ctx)
	defer cleanupInstance()

	tableProv, err := NewBigtableProvisioner(testutil.Config, TableResourceType)
	require.NoError(t, err)

	tableName := fmt.Sprintf("table_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	// Create table
	tableProperties := map[string]interface{}{
		"instance": instanceName,
		"name":     tableName,
		"columnFamilies": map[string]interface{}{
			"cf1": map[string]interface{}{
				"gcRule": map[string]interface{}{
					"maxNumVersions": 3,
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

	var tableNativeID string
	if createResult.ProgressResult.OperationStatus == resource.OperationStatusSuccess {
		tableNativeID = createResult.ProgressResult.NativeID
	} else if createResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
		statusResult, err := testutil.PollUntilComplete(t, ctx, tableProv,
			createResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
				MaxAttempts:   30,
				CheckInterval: 5 * time.Second,
				ResourceType:  TableResourceType,
				OperationName: "Create",
			})
		require.NoError(t, err)
		tableNativeID = statusResult.ProgressResult.NativeID
	}

	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     tableNativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = tableProv.Delete(ctx, deleteReq)
	}()

	// Read table
	readReq := &resource.ReadRequest{
		NativeID:     tableNativeID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := tableProv.Read(ctx, readReq)
	require.NoError(t, err)
	require.NotNil(t, readResult)
	require.NotEmpty(t, readResult.Properties)

	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err)

	assert.Contains(t, props["name"].(string), tableName)

	if columnFamilies, ok := props["columnFamilies"].(map[string]interface{}); ok {
		assert.Contains(t, columnFamilies, "cf1")
	}
	t.Logf("Table read successfully")
}

// TestTable_Update_NotUpdatable_Integration tests that table updates return NotUpdatable.
func TestTable_Update_NotUpdatable_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	instanceName, _, cleanupInstance := createTestInstance(t, ctx)
	defer cleanupInstance()

	tableProv, err := NewBigtableProvisioner(testutil.Config, TableResourceType)
	require.NoError(t, err)

	tableName := fmt.Sprintf("table_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	// Create table
	tableProperties := map[string]interface{}{
		"instance": instanceName,
		"name":     tableName,
		"columnFamilies": map[string]interface{}{
			"cf1": map[string]interface{}{
				"gcRule": map[string]interface{}{
					"maxNumVersions": 3,
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

	var tableNativeID string
	if createResult.ProgressResult.OperationStatus == resource.OperationStatusSuccess {
		tableNativeID = createResult.ProgressResult.NativeID
	} else if createResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
		statusResult, err := testutil.PollUntilComplete(t, ctx, tableProv,
			createResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
				MaxAttempts:   30,
				CheckInterval: 5 * time.Second,
				ResourceType:  TableResourceType,
				OperationName: "Create",
			})
		require.NoError(t, err)
		tableNativeID = statusResult.ProgressResult.NativeID
	}

	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     tableNativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = tableProv.Delete(ctx, deleteReq)
	}()

	// Attempt to update
	updatedProperties := map[string]interface{}{
		"instance": instanceName,
		"name":     tableName,
		"columnFamilies": map[string]interface{}{
			"cf3": map[string]interface{}{
				"gcRule": map[string]interface{}{
					"maxNumVersions": 5,
				},
			},
		},
	}

	updatedPropsJSON, err := json.Marshal(updatedProperties)
	require.NoError(t, err)

	updateReq := &resource.UpdateRequest{
		NativeID:          tableNativeID,
		ResourceType:      TableResourceType,
		DesiredProperties: updatedPropsJSON,
		TargetConfig:      testutil.TargetConfig,
	}

	updateResult, err := tableProv.Update(ctx, updateReq)
	require.NoError(t, err)
	require.NotNil(t, updateResult)
	require.Equal(t, resource.OperationStatusFailure, updateResult.ProgressResult.OperationStatus)
	require.Equal(t, resource.OperationErrorCodeNotUpdatable, updateResult.ProgressResult.ErrorCode)
	t.Logf("Update correctly returned NotUpdatable")
}

// TestTable_Delete_Integration tests deleting a Bigtable table.
func TestTable_Delete_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	instanceName, _, cleanupInstance := createTestInstance(t, ctx)
	defer cleanupInstance()

	tableProv, err := NewBigtableProvisioner(testutil.Config, TableResourceType)
	require.NoError(t, err)

	tableName := fmt.Sprintf("table_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	// Create table
	tableProperties := map[string]interface{}{
		"instance": instanceName,
		"name":     tableName,
		"columnFamilies": map[string]interface{}{
			"cf1": map[string]interface{}{
				"gcRule": map[string]interface{}{
					"maxNumVersions": 3,
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

	var tableNativeID string
	if createResult.ProgressResult.OperationStatus == resource.OperationStatusSuccess {
		tableNativeID = createResult.ProgressResult.NativeID
	} else if createResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
		statusResult, err := testutil.PollUntilComplete(t, ctx, tableProv,
			createResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
				MaxAttempts:   30,
				CheckInterval: 5 * time.Second,
				ResourceType:  TableResourceType,
				OperationName: "Create",
			})
		require.NoError(t, err)
		tableNativeID = statusResult.ProgressResult.NativeID
	}

	// Delete table
	deleteReq := &resource.DeleteRequest{
		NativeID:     tableNativeID,
		TargetConfig: testutil.TargetConfig,
	}

	deleteResult, err := tableProv.Delete(ctx, deleteReq)
	require.NoError(t, err)
	require.NotNil(t, deleteResult)

	if deleteResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
		statusResult, err := testutil.PollUntilComplete(t, ctx, tableProv,
			deleteResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
				MaxAttempts:   30,
				CheckInterval: 5 * time.Second,
				ResourceType:  TableResourceType,
				OperationName: "Delete",
			})
		require.NoError(t, err)
		require.Equal(t, resource.OperationStatusSuccess, statusResult.ProgressResult.OperationStatus)
	} else {
		require.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus)
	}
	t.Logf("Table deleted: %s", tableNativeID)

	// Verify it's deleted
	readReq := &resource.ReadRequest{
		NativeID:     tableNativeID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := tableProv.Read(ctx, readReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode)
	t.Logf("Verified table is deleted (not found)")
}

// TestTable_Delete_NotFound_Integration tests deleting a non-existent table.
func TestTable_Delete_NotFound_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	tableProv, err := NewBigtableProvisioner(testutil.Config, TableResourceType)
	require.NoError(t, err)

	nonExistentID := fmt.Sprintf("projects/%s/instances/nonexistent/tables/nonexistent_%s",
		testutil.Project, strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	deleteReq := &resource.DeleteRequest{
		NativeID:     nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	deleteResult, err := tableProv.Delete(ctx, deleteReq)
	require.NoError(t, err)
	require.NotNil(t, deleteResult)
	// Bigtable returns Failure for non-existent resources
	require.Equal(t, resource.OperationStatusFailure, deleteResult.ProgressResult.OperationStatus)
	t.Logf("Delete non-existent table returned status: %s", deleteResult.ProgressResult.OperationStatus)
}

// TestTable_List_Integration tests listing Bigtable tables.
func TestTable_List_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	instanceName, _, cleanupInstance := createTestInstance(t, ctx)
	defer cleanupInstance()

	tableProv, err := NewBigtableProvisioner(testutil.Config, TableResourceType)
	require.NoError(t, err)

	tableName := fmt.Sprintf("table_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	// Create table
	tableProperties := map[string]interface{}{
		"instance": instanceName,
		"name":     tableName,
		"columnFamilies": map[string]interface{}{
			"cf1": map[string]interface{}{
				"gcRule": map[string]interface{}{
					"maxNumVersions": 3,
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

	var tableNativeID string
	if createResult.ProgressResult.OperationStatus == resource.OperationStatusSuccess {
		tableNativeID = createResult.ProgressResult.NativeID
	} else if createResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
		statusResult, err := testutil.PollUntilComplete(t, ctx, tableProv,
			createResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
				MaxAttempts:   30,
				CheckInterval: 5 * time.Second,
				ResourceType:  TableResourceType,
				OperationName: "Create",
			})
		require.NoError(t, err)
		tableNativeID = statusResult.ProgressResult.NativeID
	}

	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     tableNativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = tableProv.Delete(ctx, deleteReq)
	}()

	// List tables
	listReq := &resource.ListRequest{
		ResourceType: TableResourceType,
		TargetConfig: testutil.TargetConfig,
		AdditionalProperties: map[string]string{
			"instance": instanceName,
		},
	}

	listResult, err := tableProv.List(ctx, listReq)
	require.NoError(t, err)
	require.NotNil(t, listResult)
	require.NotEmpty(t, listResult.NativeIDs)
	t.Logf("Listed %d tables in instance", len(listResult.NativeIDs))

	// Verify our table is in the list
	found := false
	for _, id := range listResult.NativeIDs {
		if id == tableNativeID {
			found = true
			break
		}
	}
	assert.True(t, found, "Created table should be in the list")
}

// TestTable_Read_NotFound_Integration tests reading a non-existent table.
func TestTable_Read_NotFound_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	tableProv, err := NewBigtableProvisioner(testutil.Config, TableResourceType)
	require.NoError(t, err)

	nonExistentID := fmt.Sprintf("projects/%s/instances/nonexistent/tables/nonexistent_%s",
		testutil.Project, strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	readReq := &resource.ReadRequest{
		NativeID:     nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := tableProv.Read(ctx, readReq)
	require.NoError(t, err)
	require.NotNil(t, readResult)
	require.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode)
	t.Logf("Correctly handled read of non-existent table")
}

// TestTable_CreateInvalid_Integration tests error handling for invalid create requests.
func TestTable_CreateInvalid_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	tableProv, err := NewBigtableProvisioner(testutil.Config, TableResourceType)
	require.NoError(t, err)

	// Test with missing instance
	tableProperties := map[string]interface{}{
		"name": "test_table",
		"columnFamilies": map[string]interface{}{
			"cf1": map[string]interface{}{},
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
	require.Equal(t, resource.OperationStatusFailure, createResult.ProgressResult.OperationStatus)
	require.Equal(t, resource.OperationErrorCodeInvalidRequest, createResult.ProgressResult.ErrorCode)
	assert.Contains(t, createResult.ProgressResult.StatusMessage, "instance")
	t.Logf("Correctly rejected create with missing instance")
}
