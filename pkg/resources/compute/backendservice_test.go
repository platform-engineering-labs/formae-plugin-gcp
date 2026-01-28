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

// TestBackendServiceCreate tests the creation, reading, updating, and deletion of a global GCP Backend Service
func TestBackendServiceCreate(t *testing.T) {
	testID := uuid.New().String()[:8]

	// Create provisioners
	healthCheckProvisioner, err := NewComputeProvisioner(testutil.Config, HealthCheckResourceType)
	require.NoError(t, err, "Failed to create health check provisioner")

	backendServiceProvisioner, err := NewComputeProvisioner(testutil.Config, BackendServiceResourceType)
	require.NoError(t, err, "Failed to create backend service provisioner")

	ctx := context.Background()

	// First, create a health check (required dependency for backend service)
	healthCheckName := fmt.Sprintf("formae-test-hc-%s", testID)
	t.Logf("Creating prerequisite health check: %s", healthCheckName)

	hcProperties := map[string]interface{}{
		"name":               healthCheckName,
		"description":        "Health check for backend service test",
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

	hcPropertiesJSON, err := json.Marshal(hcProperties)
	require.NoError(t, err, "Failed to marshal health check properties")

	hcCreateReq := &resource.CreateRequest{
		ResourceType: HealthCheckResourceType,
		Properties:   hcPropertiesJSON,
		TargetConfig: testutil.TargetConfig,
	}

	hcCreateResult, err := healthCheckProvisioner.Create(ctx, hcCreateReq)
	require.NoError(t, err, "Health check creation should not return error")

	hcStatusResult, err := testutil.WaitForCreate(t, ctx, healthCheckProvisioner, hcCreateResult, testutil.TargetConfig, HealthCheckResourceType)
	require.NoError(t, err, "Health check creation should complete successfully")

	healthCheckNativeID := hcStatusResult.ProgressResult.NativeID
	healthCheckSelfLink := fmt.Sprintf("https://www.googleapis.com/compute/v1/%s", healthCheckNativeID)
	t.Logf("Health check created: %s", healthCheckNativeID)

	// Cleanup health check at the end
	defer func() {
		t.Log("Cleaning up health check...")
		deleteReq := &resource.DeleteRequest{
			NativeID:     healthCheckNativeID,
			TargetConfig: testutil.TargetConfig,
		}
		deleteResult, err := healthCheckProvisioner.Delete(ctx, deleteReq)
		if err != nil {
			t.Logf("Warning: Failed to delete health check: %v", err)
			return
		}
		_, err = testutil.WaitForDelete(t, ctx, healthCheckProvisioner, deleteResult, testutil.TargetConfig, HealthCheckResourceType)
		if err != nil {
			t.Logf("Warning: Health check deletion did not complete: %v", err)
		}
	}()

	// Now create the backend service
	backendServiceName := fmt.Sprintf("formae-test-bs-%s", testID)
	t.Logf("Creating test backend service: %s", backendServiceName)

	var backendServiceNativeID string

	// Test Create operation
	t.Run("Create", func(t *testing.T) {
		bsProperties := map[string]interface{}{
			"name":                backendServiceName,
			"description":         "Test backend service created by Formae integration test",
			"protocol":            "HTTP",
			"portName":            "http",
			"timeoutSec":          30,
			"loadBalancingScheme": "EXTERNAL",
			"healthChecks":        []string{healthCheckSelfLink},
			"connectionDraining": map[string]interface{}{
				"drainingTimeoutSec": 300,
			},
			"logConfig": map[string]interface{}{
				"enable":     true,
				"sampleRate": 1.0,
			},
		}

		bsPropertiesJSON, err := json.Marshal(bsProperties)
		require.NoError(t, err, "Failed to marshal backend service properties")

		createReq := &resource.CreateRequest{
			ResourceType: BackendServiceResourceType,
			Properties:   bsPropertiesJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := backendServiceProvisioner.Create(ctx, createReq)
		require.NoError(t, err, "Create operation should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation, "Operation should be Create")
		assert.Equal(t, resource.OperationStatusInProgress, createResult.ProgressResult.OperationStatus, "Should be in progress")

		t.Logf("Backend service creation initiated with RequestID: %s", createResult.ProgressResult.RequestID)

		// Wait for creation to complete
		statusResult, err := testutil.WaitForCreate(t, ctx, backendServiceProvisioner, createResult, testutil.TargetConfig, BackendServiceResourceType)
		require.NoError(t, err, "Backend service creation should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")

		backendServiceNativeID = statusResult.ProgressResult.NativeID
		t.Logf("Backend service created with native ID: %s", backendServiceNativeID)

		// Verify native ID format
		expectedNativeID := fmt.Sprintf("projects/%s/global/backendServices/%s", testutil.Project, backendServiceName)
		assert.Equal(t, expectedNativeID, backendServiceNativeID, "Native ID should match expected format")
	})

	// Test Read operation
	t.Run("Read", func(t *testing.T) {
		readReq := &resource.ReadRequest{
			NativeID:     backendServiceNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: BackendServiceResourceType,
		}

		readResult, err := backendServiceProvisioner.Read(ctx, readReq)
		require.NoError(t, err, "Read operation should not return error")
		require.NotNil(t, readResult, "Read result should not be nil")
		require.Empty(t, readResult.ErrorCode, "Read should not have error code")
		require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

		// Verify properties
		var readProps map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &readProps)
		require.NoError(t, err, "Failed to unmarshal read properties")

		assert.Equal(t, backendServiceName, readProps["name"], "Backend service name should match")
		assert.Equal(t, "Test backend service created by Formae integration test", readProps["description"], "Description should match")
		assert.Equal(t, "HTTP", readProps["protocol"], "Protocol should match")
		t.Logf("Read backend service properties successfully")
	})

	// Test Update operation
	t.Run("Update", func(t *testing.T) {
		// Read current properties first to get fingerprint
		readReq := &resource.ReadRequest{
			NativeID:     backendServiceNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: BackendServiceResourceType,
		}
		readResult, err := backendServiceProvisioner.Read(ctx, readReq)
		require.NoError(t, err, "Read before update should not return error")

		var currentProps map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &currentProps)
		require.NoError(t, err, "Failed to unmarshal current properties")

		// Update properties (fingerprint is needed for optimistic locking)
		currentProps["description"] = "Updated description for test backend service"
		currentProps["timeoutSec"] = float64(60)

		updatedPropsJSON, err := json.Marshal(currentProps)
		require.NoError(t, err, "Failed to marshal updated properties")

		updateReq := &resource.UpdateRequest{
			NativeID:          backendServiceNativeID,
			ResourceType:      BackendServiceResourceType,
			DesiredProperties: updatedPropsJSON,
			TargetConfig:      testutil.TargetConfig,
		}

		updateResult, err := backendServiceProvisioner.Update(ctx, updateReq)
		require.NoError(t, err, "Update operation should not return error")
		require.NotNil(t, updateResult, "Update result should not be nil")

		// Wait for update to complete
		statusResult, err := testutil.WaitForUpdate(t, ctx, backendServiceProvisioner, updateResult, testutil.TargetConfig, BackendServiceResourceType)
		require.NoError(t, err, "Backend service update should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")

		t.Logf("Backend service updated successfully")

		// Verify update
		readResult, err = backendServiceProvisioner.Read(ctx, readReq)
		require.NoError(t, err, "Read after update should not return error")

		var updatedProps map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &updatedProps)
		require.NoError(t, err, "Failed to unmarshal updated properties")

		assert.Equal(t, "Updated description for test backend service", updatedProps["description"], "Description should be updated")
		assert.Equal(t, float64(60), updatedProps["timeoutSec"], "Timeout should be updated")
	})

	// Test List operation
	t.Run("List", func(t *testing.T) {
		listReq := &resource.ListRequest{
			ResourceType: BackendServiceResourceType,
			TargetConfig: testutil.TargetConfig,
		}

		listResult, err := backendServiceProvisioner.List(ctx, listReq)
		require.NoError(t, err, "List operation should not return error")
		require.NotNil(t, listResult, "List result should not be nil")
		assert.NotEmpty(t, listResult.NativeIDs, "NativeIDs list should not be empty")

		// Check if our test backend service is in the list
		found := false
		for _, id := range listResult.NativeIDs {
			if id == backendServiceNativeID {
				found = true
				break
			}
		}
		assert.True(t, found, "Test backend service should be in the list")
		t.Logf("Successfully found test backend service in the list")
	})

	// Test Delete operation
	t.Run("Delete", func(t *testing.T) {
		deleteReq := &resource.DeleteRequest{
			NativeID:     backendServiceNativeID,
			TargetConfig: testutil.TargetConfig,
		}

		deleteResult, err := backendServiceProvisioner.Delete(ctx, deleteReq)
		require.NoError(t, err, "Delete operation should not return error")
		require.NotNil(t, deleteResult, "Delete result should not be nil")

		// Wait for deletion to complete
		statusResult, err := testutil.WaitForDelete(t, ctx, backendServiceProvisioner, deleteResult, testutil.TargetConfig, BackendServiceResourceType)
		require.NoError(t, err, "Backend service deletion should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")

		t.Logf("Backend service deleted successfully")
	})

	// Verify deletion
	t.Run("VerifyDeleted", func(t *testing.T) {
		time.Sleep(2 * time.Second)

		readReq := &resource.ReadRequest{
			NativeID:     backendServiceNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: BackendServiceResourceType,
		}

		readResult, err := backendServiceProvisioner.Read(ctx, readReq)

		if err == nil {
			require.NotNil(t, readResult, "Read result should not be nil")
			assert.NotEmpty(t, readResult.ErrorCode, "Should have error code for deleted resource")
		}

		t.Logf("Verified backend service was deleted")
	})
}
