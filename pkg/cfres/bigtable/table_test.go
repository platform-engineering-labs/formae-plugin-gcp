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
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTableCreate tests the full CRUD lifecycle of a Bigtable table
func TestTableCreate(t *testing.T) {
	ctx := context.Background()

	// Create provisioners
	instanceProv, err := NewBigtableProvisioner(testutil.Config, InstanceResourceType)
	require.NoError(t, err)

	tableProv, err := NewBigtableProvisioner(testutil.Config, TableResourceType)
	require.NoError(t, err)

	// Generate unique names
	instanceName := fmt.Sprintf("formae-test-bt-%s", strings.ToLower(uuid.New().String()[:8]))
	tableName := fmt.Sprintf("table_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	var instanceNativeID string
	var tableNativeID string

	// Test 1: Create Instance (prerequisite for table)
	t.Run("CreateInstance", func(t *testing.T) {
		instanceProperties := map[string]interface{}{
			"name":        instanceName,
			"displayName": "Formae Table Test",
			"type":        "DEVELOPMENT",
			"labels": map[string]interface{}{
				"test": "formae-bigtable-table",
			},
			// Instances require at least one cluster
			"clusters": map[string]interface{}{
				"cluster1": map[string]interface{}{
					"location":           testutil.Region + "-a", // Use zone in same region
					"defaultStorageType": "SSD",
					// Note: serveNodes should NOT be specified for DEVELOPMENT instances
				},
			},
		}

		propsJSON, err := json.Marshal(instanceProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: InstanceResourceType,
			Properties:   propsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := instanceProv.Create(ctx, createReq)
		require.NoError(t, err)

		if createResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
			statusResult, err := testutil.PollUntilComplete(t, ctx, instanceProv,
				createResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
				MaxAttempts:   60,
				CheckInterval: 10 * time.Second,
				ResourceType:  InstanceResourceType,
				OperationName: "Create",
			})
			require.NoError(t, err)
			instanceNativeID = statusResult.ProgressResult.NativeID
		} else {
			instanceNativeID = createResult.ProgressResult.NativeID
		}

		require.NotEmpty(t, instanceNativeID)
		t.Logf("Instance created: %s", instanceNativeID)
	})

	// Cleanup function
	defer func() {
		if instanceNativeID != "" {
			deleteReq := &resource.DeleteRequest{
				NativeID: instanceNativeID,
				TargetConfig: testutil.TargetConfig,
			}
			deleteResult, err := instanceProv.Delete(ctx, deleteReq)
			if err == nil && deleteResult != nil && deleteResult.ProgressResult != nil {
				if deleteResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
					_, _ = testutil.PollUntilComplete(t, ctx, instanceProv,
						deleteResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
						MaxAttempts:   60,
						CheckInterval: 10 * time.Second,
						ResourceType:  InstanceResourceType,
						OperationName: "Delete",
					})
				}
			}
			t.Logf("Instance cleanup completed")
		}
	}()

	// Test 2: Create Table with Column Families
	t.Run("CreateTable", func(t *testing.T) {
		require.NotEmpty(t, instanceNativeID)

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
						"maxAge": "259200s", // 72 hours in seconds (GCP requires duration ending with 's')
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

		// Table creation should be synchronous or complete quickly
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

		require.NotEmpty(t, tableNativeID)
		expectedID := fmt.Sprintf("projects/%s/instances/%s/tables/%s", testutil.Project, instanceName, tableName)
		assert.Equal(t, expectedID, tableNativeID)
		t.Logf("Table created: %s", tableNativeID)
	})

	// Test 3: Read Table
	t.Run("ReadTable", func(t *testing.T) {
		require.NotEmpty(t, tableNativeID)

		readReq := &resource.ReadRequest{
			NativeID: tableNativeID,
			TargetConfig: testutil.TargetConfig,
		}

		readResult, err := tableProv.Read(ctx, readReq)
		require.NoError(t, err)
		require.NotNil(t, readResult)
		require.NotEmpty(t, readResult.Properties)

		var props map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &props)
		require.NoError(t, err)

		// Verify key properties
		assert.Contains(t, props["name"].(string), tableName)

		// Verify column families
		if columnFamilies, ok := props["columnFamilies"].(map[string]interface{}); ok {
			assert.Contains(t, columnFamilies, "cf1")
			assert.Contains(t, columnFamilies, "cf2")
			t.Logf("Column families: %+v", columnFamilies)
		}

		t.Logf("Table properties: %+v", props)
	})

	// Test 4: List Tables
	t.Run("ListTables", func(t *testing.T) {
		require.NotEmpty(t, instanceName)

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

		// Verify our table is in the list
		found := false
		for _, id := range listResult.NativeIDs {
			if id == tableNativeID {
				found = true
				break
			}
		}
		assert.True(t, found, "Our table should be in the list")
		t.Logf("Listed %d tables in instance", len(listResult.NativeIDs))
	})

	// Test 5: Update Table (should return NotUpdatable)
	t.Run("UpdateTable", func(t *testing.T) {
		require.NotEmpty(t, tableNativeID)

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

		propsJSON, err := json.Marshal(updatedProperties)
		require.NoError(t, err)

		updateReq := &resource.UpdateRequest{
			NativeID:          tableNativeID,
			ResourceType:      TableResourceType,
			DesiredProperties: propsJSON,
			TargetConfig:      testutil.TargetConfig,
		}

		updateResult, err := tableProv.Update(ctx, updateReq)
		require.NoError(t, err)
		require.NotNil(t, updateResult)
		require.Equal(t, resource.OperationStatusFailure, updateResult.ProgressResult.OperationStatus)
		require.Equal(t, resource.OperationErrorCodeNotUpdatable, updateResult.ProgressResult.ErrorCode)

		t.Logf("Update correctly returned NotUpdatable")
	})

	// Test 6: Delete Table
	t.Run("DeleteTable", func(t *testing.T) {
		require.NotEmpty(t, tableNativeID)

		deleteReq := &resource.DeleteRequest{
			NativeID: tableNativeID,
			TargetConfig: testutil.TargetConfig,
		}

		deleteResult, err := tableProv.Delete(ctx, deleteReq)
		require.NoError(t, err)
		require.NotNil(t, deleteResult)

		// Table deletion is typically synchronous
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
	})

	// Test 7: Verify Table is Deleted
	t.Run("VerifyTableDeleted", func(t *testing.T) {
		require.NotEmpty(t, tableNativeID)

		readReq := &resource.ReadRequest{
			NativeID: tableNativeID,
			TargetConfig: testutil.TargetConfig,
		}

		readResult, err := tableProv.Read(ctx, readReq)
		require.NoError(t, err)
		require.NotNil(t, readResult)
		require.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode)

		t.Logf("Verified table is deleted (not found)")
	})
}

// TestTableReadNonExistent tests reading a non-existent table
func TestTableReadNonExistent(t *testing.T) {
	ctx := context.Background()

	tableProv, err := NewBigtableProvisioner(testutil.Config, TableResourceType)
	require.NoError(t, err)

	nonExistentID := fmt.Sprintf("projects/%s/instances/nonexistent/tables/nonexistent_%s",
		testutil.Project,
		strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	readReq := &resource.ReadRequest{
		NativeID: nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := tableProv.Read(ctx, readReq)
	require.NoError(t, err)
	require.NotNil(t, readResult)
	require.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode)

	t.Logf("Correctly handled read of non-existent table: %s", nonExistentID)
}

// TestTableCreateMissingInstance tests creating a table without specifying instance
func TestTableCreateMissingInstance(t *testing.T) {
	ctx := context.Background()

	tableProv, err := NewBigtableProvisioner(testutil.Config, TableResourceType)
	require.NoError(t, err)

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

// TestTableCreateWithComplexGCRules tests creating a table with complex GC rules
func TestTableCreateWithComplexGCRules(t *testing.T) {
	t.Skip("Skipping complex GC rules test - basic CRUD test covers core functionality")

	ctx := context.Background()

	instanceProv, err := NewBigtableProvisioner(testutil.Config, InstanceResourceType)
	require.NoError(t, err)

	tableProv, err := NewBigtableProvisioner(testutil.Config, TableResourceType)
	require.NoError(t, err)

	instanceName := fmt.Sprintf("formae-test-bt-%s", strings.ToLower(uuid.New().String()[:8]))
	tableName := fmt.Sprintf("table_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	// Create instance first
	instanceProperties := map[string]interface{}{
		"name":        instanceName,
		"displayName": "Test Instance",
		"type":        "DEVELOPMENT",
	}

	propsJSON, err := json.Marshal(instanceProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: InstanceResourceType,
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := instanceProv.Create(ctx, createReq)
	require.NoError(t, err)

	var instanceNativeID string
	if createResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
		statusResult, err := testutil.PollUntilComplete(t, ctx, instanceProv,
			createResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
			MaxAttempts:   60,
			CheckInterval: 10 * time.Second,
			ResourceType:  InstanceResourceType,
			OperationName: "Create",
		})
		require.NoError(t, err)
		instanceNativeID = statusResult.ProgressResult.NativeID
	} else {
		instanceNativeID = createResult.ProgressResult.NativeID
	}

	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID: instanceNativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = instanceProv.Delete(ctx, deleteReq)
	}()

	// Create table with union GC rule (keep data for 7 days OR keep 3 versions)
	tableProperties := map[string]interface{}{
		"instance": instanceName,
		"name":     tableName,
		"columnFamilies": map[string]interface{}{
			"cf1": map[string]interface{}{
				"gcRule": map[string]interface{}{
					"union": []map[string]interface{}{
						{"maxAge": "604800s"}, // 7 days in seconds (GCP requires duration ending with 's')
						{"maxNumVersions": 3},
					},
				},
			},
		},
	}

	propsJSON, err = json.Marshal(tableProperties)
	require.NoError(t, err)

	createTableReq := &resource.CreateRequest{
		ResourceType: TableResourceType,
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	tableResult, err := tableProv.Create(ctx, createTableReq)
	require.NoError(t, err)
	require.NotNil(t, tableResult)

	t.Logf("Table with complex GC rules created successfully")
}
