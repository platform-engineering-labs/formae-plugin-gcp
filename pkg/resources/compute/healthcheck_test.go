// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration

package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/testutil"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHealthCheckCreate tests the creation, reading, updating, and deletion of a global GCP Health Check
func TestHealthCheckCreate(t *testing.T) {
	// Create provisioner instance
	healthCheck, err := NewComputeProvisioner(testutil.Config, HealthCheckResourceType)
	require.NoError(t, err, "Failed to create health check provisioner")

	// Generate unique name
	healthCheckName := fmt.Sprintf("formae-test-hc-%s", uuid.New().String()[:8])
	t.Logf("Creating test health check: %s", healthCheckName)

	ctx := context.Background()

	// Prepare health check properties
	properties := map[string]interface{}{
		"name":               healthCheckName,
		"description":        "Test health check created by Formae integration test",
		"type":               "HTTP",
		"checkIntervalSec":   10,
		"timeoutSec":         5,
		"healthyThreshold":   2,
		"unhealthyThreshold": 3,
		"httpHealthCheck": map[string]interface{}{
			"port":        80,
			"requestPath": "/health",
		},
	}

	propertiesJSON, err := json.Marshal(properties)
	require.NoError(t, err, "Failed to marshal properties")

	// Create request
	createReq := &resource.CreateRequest{
		ResourceType: HealthCheckResourceType,
		Properties:   propertiesJSON,
		TargetConfig: testutil.TargetConfig,
	}

	var nativeID string

	// Test Create operation
	t.Run("Create", func(t *testing.T) {
		createResult, err := healthCheck.Create(ctx, createReq)
		require.NoError(t, err, "Create operation should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

		// Log details if operation failed immediately
		if createResult.ProgressResult.OperationStatus == resource.OperationStatusFailure {
			t.Logf("Create failed with error code: %s, message: %s",
				createResult.ProgressResult.ErrorCode,
				createResult.ProgressResult.StatusMessage)
		}

		assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation, "Operation should be Create")
		assert.Equal(t, resource.OperationStatusInProgress, createResult.ProgressResult.OperationStatus, "Should be in progress")
		require.NotEmpty(t, createResult.ProgressResult.RequestID, "RequestID should be set")

		t.Logf("Health check creation initiated with RequestID: %s", createResult.ProgressResult.RequestID)

		// Wait for creation to complete
		statusResult, err := testutil.WaitForCreate(t, ctx, healthCheck, createResult, testutil.TargetConfig, HealthCheckResourceType)
		require.NoError(t, err, "Health check creation should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")

		nativeID = statusResult.ProgressResult.NativeID
		t.Logf("Health check created with native ID: %s", nativeID)

		// Verify native ID format
		expectedNativeID := fmt.Sprintf("projects/%s/global/healthChecks/%s", testutil.Project, healthCheckName)
		assert.Equal(t, expectedNativeID, nativeID, "Native ID should match expected format")
	})

	// Test Read operation
	t.Run("Read", func(t *testing.T) {
		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: HealthCheckResourceType,
		}

		readResult, err := healthCheck.Read(ctx, readReq)
		require.NoError(t, err, "Read operation should not return error")
		require.NotNil(t, readResult, "Read result should not be nil")
		require.Empty(t, readResult.ErrorCode, "Read should not have error code")
		require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

		// Verify properties
		var readProps map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &readProps)
		require.NoError(t, err, "Failed to unmarshal read properties")

		assert.Equal(t, healthCheckName, readProps["name"], "Health check name should match")
		assert.Equal(t, "Test health check created by Formae integration test", readProps["description"], "Description should match")
		t.Logf("Read health check properties successfully")
	})

	// Test Update operation
	t.Run("Update", func(t *testing.T) {
		// Read current properties first
		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: HealthCheckResourceType,
		}
		readResult, err := healthCheck.Read(ctx, readReq)
		require.NoError(t, err, "Read before update should not return error")

		var currentProps map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &currentProps)
		require.NoError(t, err, "Failed to unmarshal current properties")

		// Update properties
		currentProps["description"] = "Updated description for test health check"
		currentProps["checkIntervalSec"] = float64(15)

		updatedPropsJSON, err := json.Marshal(currentProps)
		require.NoError(t, err, "Failed to marshal updated properties")

		updateReq := &resource.UpdateRequest{
			NativeID:          nativeID,
			ResourceType:      HealthCheckResourceType,
			DesiredProperties: updatedPropsJSON,
			TargetConfig:      testutil.TargetConfig,
		}

		updateResult, err := healthCheck.Update(ctx, updateReq)
		require.NoError(t, err, "Update operation should not return error")
		require.NotNil(t, updateResult, "Update result should not be nil")
		require.NotNil(t, updateResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationUpdate, updateResult.ProgressResult.Operation, "Operation should be Update")

		// Wait for update to complete
		statusResult, err := testutil.WaitForUpdate(t, ctx, healthCheck, updateResult, testutil.TargetConfig, HealthCheckResourceType)
		require.NoError(t, err, "Health check update should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")

		t.Logf("Health check updated successfully")

		// Verify update
		readResult, err = healthCheck.Read(ctx, readReq)
		require.NoError(t, err, "Read after update should not return error")

		var updatedProps map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &updatedProps)
		require.NoError(t, err, "Failed to unmarshal updated properties")

		assert.Equal(t, "Updated description for test health check", updatedProps["description"], "Description should be updated")
		assert.Equal(t, float64(15), updatedProps["checkIntervalSec"], "Check interval should be updated")
	})

	// Test List operation
	t.Run("List", func(t *testing.T) {
		listReq := &resource.ListRequest{
			ResourceType: HealthCheckResourceType,
			TargetConfig: testutil.TargetConfig,
		}

		listResult, err := healthCheck.List(ctx, listReq)
		require.NoError(t, err, "List operation should not return error")
		require.NotNil(t, listResult, "List result should not be nil")
		assert.NotEmpty(t, listResult.NativeIDs, "Resources list should not be empty")

		// Check if our test health check is in the list
		found := false
		for _, id := range listResult.NativeIDs {
			if id == nativeID {
				found = true
				break
			}
		}
		assert.True(t, found, "Test health check should be in the list")
		t.Logf("Successfully found test health check in the list")
	})

	// Test Delete operation
	t.Run("Delete", func(t *testing.T) {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		deleteResult, err := healthCheck.Delete(ctx, deleteReq)
		require.NoError(t, err, "Delete operation should not return error")
		require.NotNil(t, deleteResult, "Delete result should not be nil")
		require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationDelete, deleteResult.ProgressResult.Operation, "Operation should be Delete")

		// Wait for deletion to complete
		statusResult, err := testutil.WaitForDelete(t, ctx, healthCheck, deleteResult, testutil.TargetConfig, HealthCheckResourceType)
		require.NoError(t, err, "Health check deletion should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")

		t.Logf("Health check deleted successfully")
	})

	// Verify deletion
	t.Run("VerifyDeleted", func(t *testing.T) {
		time.Sleep(2 * time.Second)

		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: HealthCheckResourceType,
		}

		readResult, err := healthCheck.Read(ctx, readReq)

		if err == nil {
			require.NotNil(t, readResult, "Read result should not be nil")
			assert.NotEmpty(t, readResult.ErrorCode, "Should have error code for deleted resource")
		}

		t.Logf("Verified health check was deleted")
	})
}

// TestRegionHealthCheckCreate tests the creation and deletion of a regional GCP Health Check
func TestRegionHealthCheckCreate(t *testing.T) {
	// Create provisioner instance
	healthCheck, err := NewComputeProvisioner(testutil.Config, RegionHealthCheckResourceType)
	require.NoError(t, err, "Failed to create region health check provisioner")

	// Generate unique name
	healthCheckName := fmt.Sprintf("formae-test-rhc-%s", uuid.New().String()[:8])
	t.Logf("Creating test region health check: %s", healthCheckName)

	ctx := context.Background()

	// Prepare health check properties
	properties := map[string]interface{}{
		"name":               healthCheckName,
		"region":             testutil.Region,
		"description":        "Test regional health check created by Formae integration test",
		"type":               "TCP",
		"checkIntervalSec":   10,
		"timeoutSec":         5,
		"healthyThreshold":   2,
		"unhealthyThreshold": 3,
		"tcpHealthCheck": map[string]interface{}{
			"port": 8080,
		},
	}

	propertiesJSON, err := json.Marshal(properties)
	require.NoError(t, err, "Failed to marshal properties")

	// Create request
	createReq := &resource.CreateRequest{
		ResourceType: RegionHealthCheckResourceType,
		Properties:   propertiesJSON,
		TargetConfig: testutil.TargetConfig,
	}

	var nativeID string

	// Test Create operation
	t.Run("Create", func(t *testing.T) {
		createResult, err := healthCheck.Create(ctx, createReq)
		require.NoError(t, err, "Create operation should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation, "Operation should be Create")
		assert.Equal(t, resource.OperationStatusInProgress, createResult.ProgressResult.OperationStatus, "Should be in progress")

		t.Logf("Region health check creation initiated with RequestID: %s", createResult.ProgressResult.RequestID)

		// Wait for creation to complete
		statusResult, err := testutil.WaitForCreate(t, ctx, healthCheck, createResult, testutil.TargetConfig, RegionHealthCheckResourceType)
		require.NoError(t, err, "Region health check creation should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")

		nativeID = statusResult.ProgressResult.NativeID
		t.Logf("Region health check created with native ID: %s", nativeID)

		// Verify native ID format
		expectedNativeID := fmt.Sprintf("projects/%s/regions/%s/healthChecks/%s", testutil.Project, testutil.Region, healthCheckName)
		assert.Equal(t, expectedNativeID, nativeID, "Native ID should match expected format")
	})

	// Test Read operation
	t.Run("Read", func(t *testing.T) {
		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: RegionHealthCheckResourceType,
		}

		readResult, err := healthCheck.Read(ctx, readReq)
		require.NoError(t, err, "Read operation should not return error")
		require.NotNil(t, readResult, "Read result should not be nil")
		require.Empty(t, readResult.ErrorCode, "Read should not have error code")
		require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

		var readProps map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &readProps)
		require.NoError(t, err, "Failed to unmarshal read properties")

		assert.Equal(t, healthCheckName, readProps["name"], "Health check name should match")
		t.Logf("Read region health check properties successfully")
	})

	// Test Delete operation
	t.Run("Delete", func(t *testing.T) {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		deleteResult, err := healthCheck.Delete(ctx, deleteReq)
		require.NoError(t, err, "Delete operation should not return error")
		require.NotNil(t, deleteResult, "Delete result should not be nil")

		// Wait for deletion to complete
		statusResult, err := testutil.WaitForDelete(t, ctx, healthCheck, deleteResult, testutil.TargetConfig, RegionHealthCheckResourceType)
		require.NoError(t, err, "Region health check deletion should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")

		t.Logf("Region health check deleted successfully")
	})
}
