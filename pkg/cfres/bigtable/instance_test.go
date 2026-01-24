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

// TestInstanceCreate tests the full CRUD lifecycle of a Bigtable instance
func TestInstanceCreate(t *testing.T) {
	// Create instance provisioner
	instanceProv, err := NewBigtableProvisioner(testutil.Config, InstanceResourceType)
	require.NoError(t, err, "Failed to create Bigtable instance provisioner")

	// Bigtable instance names must be lowercase, alphanumeric, and hyphens
	instanceName := fmt.Sprintf("formae-test-bt-%s", strings.ToLower(uuid.New().String()[:8]))
	ctx := context.Background()
	nativeID := ""

	// Test 1: Create Instance
	t.Run("CreateInstance", func(t *testing.T) {
		instanceProperties := map[string]interface{}{
			"name":        instanceName,
			"displayName": "Formae Test Instance",
			"type":        "DEVELOPMENT", // Use DEVELOPMENT for faster creation
			"labels": map[string]interface{}{
				"test":        "formae-bigtable-instance",
				"environment": "integration-test",
			},
			// For DEVELOPMENT instances, at least one cluster is still required
			"clusters": map[string]interface{}{
				"cluster1": map[string]interface{}{
					"location":           testutil.Region + "-a", // Use zone in same region
					"defaultStorageType": "SSD",
					// Note: serveNodes should NOT be specified for DEVELOPMENT instances
				},
			},
		}

		propsJSON, err := json.Marshal(instanceProperties)
		require.NoError(t, err, "Failed to marshal instance properties")

		createReq := &resource.CreateRequest{
			ResourceType: InstanceResourceType,
			Properties:   propsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := instanceProv.Create(ctx, createReq)
		require.NoError(t, err, "Create should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

		// Bigtable instance creation is asynchronous (LRO)
		if createResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
			require.NotEmpty(t, createResult.ProgressResult.RequestID, "Request ID should not be empty for async operation")
			require.NotEmpty(t, createResult.ProgressResult.NativeID, "Native ID should not be empty")

			nativeID = createResult.ProgressResult.NativeID
			t.Logf("Instance creation initiated with request ID: %s", createResult.ProgressResult.RequestID)
			t.Logf("Native ID: %s", nativeID)

			// Wait for creation to complete
			statusResult, err := testutil.PollUntilComplete(t, ctx, instanceProv, createResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
				MaxAttempts:   60,                 // 10 minutes max
				CheckInterval: 10 * time.Second, // Check every 10 seconds
				ResourceType:  InstanceResourceType,
				OperationName: "Create",
			})
			require.NoError(t, err, "Instance creation should complete successfully")
			require.NotNil(t, statusResult, "Status result should not be nil")
			require.Equal(t, resource.OperationStatusSuccess, statusResult.ProgressResult.OperationStatus)

			t.Logf("Instance created successfully: %s", nativeID)
		} else {
			// Synchronous creation (unlikely for instances)
			require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)
			require.NotEmpty(t, createResult.ProgressResult.NativeID)
			nativeID = createResult.ProgressResult.NativeID
			t.Logf("Instance created synchronously: %s", nativeID)
		}

		// Verify native ID format: projects/{project}/instances/{instance}
		expectedPrefix := fmt.Sprintf("projects/%s/instances/%s", testutil.Project, instanceName)
		assert.Equal(t, expectedPrefix, nativeID, "Native ID should match expected format")
	})

	// Test 2: Read Instance
	t.Run("ReadInstance", func(t *testing.T) {
		require.NotEmpty(t, nativeID, "Native ID should be set from create test")

		readReq := &resource.ReadRequest{
			NativeID: nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		readResult, err := instanceProv.Read(ctx, readReq)
		require.NoError(t, err, "Read should not return error")
		require.NotNil(t, readResult, "Read result should not be nil")
		require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

		t.Logf("Read instance successfully")

		// Unmarshal and verify properties
		var props map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &props)
		require.NoError(t, err, "Should be able to unmarshal resource properties")

		// Verify key properties
		assert.Equal(t, "Formae Test Instance", props["displayName"], "Display name should match")
		assert.Equal(t, "DEVELOPMENT", props["type"], "Type should match")
		assert.Equal(t, "READY", props["state"], "State should be READY")

		// Check labels
		if labels, ok := props["labels"].(map[string]interface{}); ok {
			assert.Equal(t, "formae-bigtable-instance", labels["test"], "Test label should match")
			assert.Equal(t, "integration-test", labels["environment"], "Environment label should match")
		} else {
			t.Error("Labels should be present and be a map")
		}
	})

	// Test 3: List Instances
	t.Run("ListInstances", func(t *testing.T) {
		listReq := &resource.ListRequest{
			ResourceType: InstanceResourceType,
			TargetConfig: testutil.TargetConfig,
		}

		listResult, err := instanceProv.List(ctx, listReq)
		require.NoError(t, err, "List should not return error")
		require.NotNil(t, listResult, "List result should not be nil")
		require.NotEmpty(t, listResult.NativeIDs, "List should return at least one instance")

		t.Logf("Listed %d instances", len(listResult.NativeIDs))

		// Verify our instance is in the list
		found := false
		for _, id := range listResult.NativeIDs {
			if id == nativeID {
				found = true
				t.Logf("Found our instance in list: %s", id)
				break
			}
		}
		assert.True(t, found, "Our instance should be in the list")
	})

	// Test 4: Update Instance (should return NotUpdatable)
	t.Run("UpdateInstance", func(t *testing.T) {
		require.NotEmpty(t, nativeID, "Native ID should be set from create test")

		updatedProperties := map[string]interface{}{
			"name":        instanceName,
			"displayName": "Updated Display Name",
		}

		propsJSON, err := json.Marshal(updatedProperties)
		require.NoError(t, err, "Failed to marshal updated properties")

		updateReq := &resource.UpdateRequest{
			NativeID:          nativeID,
			ResourceType:      InstanceResourceType,
			DesiredProperties: propsJSON,
			TargetConfig:      testutil.TargetConfig,
		}

		updateResult, err := instanceProv.Update(ctx, updateReq)
		require.NoError(t, err, "Update should not return error")
		require.NotNil(t, updateResult, "Update result should not be nil")
		require.Equal(t, resource.OperationStatusFailure, updateResult.ProgressResult.OperationStatus)
		require.Equal(t, resource.OperationErrorCodeNotUpdatable, updateResult.ProgressResult.ErrorCode)

		t.Logf("Update correctly returned NotUpdatable status")
	})

	// Test 5: Delete Instance
	t.Run("DeleteInstance", func(t *testing.T) {
		require.NotEmpty(t, nativeID, "Native ID should be set from create test")

		deleteReq := &resource.DeleteRequest{
			NativeID: nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		deleteResult, err := instanceProv.Delete(ctx, deleteReq)
		require.NoError(t, err, "Delete should not return error")
		require.NotNil(t, deleteResult, "Delete result should not be nil")
		require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")

		// Delete might be async or sync
		if deleteResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
			// For async deletes, we need a RequestID to poll
			if deleteResult.ProgressResult.RequestID != "" {
				t.Logf("Instance deletion initiated with request ID: %s", deleteResult.ProgressResult.RequestID)
			} else {
				// Some operations return InProgress but complete immediately without needing polling
				t.Logf("Instance deletion in progress (no polling required)")
			}

			// Only poll if we have a RequestID
			if deleteResult.ProgressResult.RequestID != "" {
				// Wait for deletion to complete
				statusResult, err := testutil.PollUntilComplete(t, ctx, instanceProv, deleteResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
					MaxAttempts:   60,
					CheckInterval: 10 * time.Second,
					ResourceType:  InstanceResourceType,
					OperationName: "Delete",
				})
				require.NoError(t, err, "Instance deletion should complete successfully")
				require.NotNil(t, statusResult, "Status result should not be nil")
				require.Equal(t, resource.OperationStatusSuccess, statusResult.ProgressResult.OperationStatus)

				t.Logf("Instance deleted successfully: %s", nativeID)
			}
		} else {
			require.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus)
			t.Logf("Instance deleted synchronously: %s", nativeID)
		}
	})

	// Test 6: Verify Instance is Deleted
	t.Run("VerifyInstanceDeleted", func(t *testing.T) {
		require.NotEmpty(t, nativeID, "Native ID should be set from create test")

		readReq := &resource.ReadRequest{
			NativeID: nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		readResult, err := instanceProv.Read(ctx, readReq)
		require.NoError(t, err, "Read should not return error")
		require.NotNil(t, readResult, "Read result should not be nil")
		require.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode,
			"Read should return NotFound error code")

		t.Logf("Verified instance is deleted (not found): %s", nativeID)
	})
}

// TestInstanceCreateProduction tests creating a production instance with clusters
func TestInstanceCreateProduction(t *testing.T) {
	t.Skip("Skipping production instance test - takes 10+ minutes and costs more")

	instanceProv, err := NewBigtableProvisioner(testutil.Config, InstanceResourceType)
	require.NoError(t, err)

	instanceName := fmt.Sprintf("formae-prod-bt-%s", strings.ToLower(uuid.New().String()[:8]))
	ctx := context.Background()

	t.Run("CreateProductionInstance", func(t *testing.T) {
		instanceProperties := map[string]interface{}{
			"name":        instanceName,
			"displayName": "Formae Prod Test",
			"type":        "PRODUCTION",
			"labels": map[string]interface{}{
				"test": "formae-bigtable-production",
			},
			"clusters": map[string]interface{}{
				"cluster1": map[string]interface{}{
					"location":           testutil.Region,
					"serveNodes":         1,
					"defaultStorageType": "SSD",
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
		require.NotNil(t, createResult)

		if createResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
			nativeID := createResult.ProgressResult.NativeID
			t.Logf("Production instance creation initiated: %s", nativeID)

			// Wait for creation (can take 10+ minutes)
			statusResult, err := testutil.PollUntilComplete(t, ctx, instanceProv, createResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
				MaxAttempts:   120,                // 20 minutes
				CheckInterval: 10 * time.Second,
				ResourceType:  InstanceResourceType,
				OperationName: "Create",
			})
			require.NoError(t, err)
			require.Equal(t, resource.OperationStatusSuccess, statusResult.ProgressResult.OperationStatus)

			t.Logf("Production instance created: %s", nativeID)

			// Cleanup
			deleteReq := &resource.DeleteRequest{
				NativeID: nativeID,
				TargetConfig: testutil.TargetConfig,
			}
			deleteResult, err := instanceProv.Delete(ctx, deleteReq)
			require.NoError(t, err)

			if deleteResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
				_, err = testutil.PollUntilComplete(t, ctx, instanceProv, deleteResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
					MaxAttempts:   120,
					CheckInterval: 10 * time.Second,
					ResourceType:  InstanceResourceType,
					OperationName: "Delete",
				})
				require.NoError(t, err)
			}
		}
	})
}

// TestInstanceInvalidCreate tests error handling
func TestInstanceInvalidCreate(t *testing.T) {
	instanceProv, err := NewBigtableProvisioner(testutil.Config, InstanceResourceType)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("CreateWithMissingName", func(t *testing.T) {
		invalidProperties := map[string]interface{}{
			"displayName": "Missing Name",
			"type":        "DEVELOPMENT",
		}

		propsJSON, err := json.Marshal(invalidProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: InstanceResourceType,
			Properties:   propsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := instanceProv.Create(ctx, createReq)
		require.NoError(t, err)
		require.Equal(t, resource.OperationStatusFailure, createResult.ProgressResult.OperationStatus)
		assert.Contains(t, createResult.ProgressResult.StatusMessage, "required")

		t.Logf("Correctly rejected create with missing name")
	})
}

// TestInstanceReadNonExistent tests reading a non-existent instance
func TestInstanceReadNonExistent(t *testing.T) {
	instanceProv, err := NewBigtableProvisioner(testutil.Config, InstanceResourceType)
	require.NoError(t, err)

	ctx := context.Background()

	nonExistentID := fmt.Sprintf("projects/%s/instances/formae-nonexistent-%s",
		testutil.Project,
		strings.ToLower(uuid.New().String()[:8]))

	readReq := &resource.ReadRequest{
		NativeID: nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := instanceProv.Read(ctx, readReq)
	require.NoError(t, err)
	require.NotNil(t, readResult)
	require.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode,
		"Read should return NotFound error code for non-existent instance")

	t.Logf("Correctly handled read of non-existent instance: %s", nonExistentID)
}

// TestInstanceList tests listing Bigtable instances without creating new resources
func TestInstanceList(t *testing.T) {
	instanceProv, err := NewBigtableProvisioner(testutil.Config, InstanceResourceType)
	require.NoError(t, err, "Failed to create Bigtable instance provisioner")

	ctx := context.Background()

	t.Run("ListAllInstances", func(t *testing.T) {
		listReq := &resource.ListRequest{
			ResourceType: InstanceResourceType,
			TargetConfig: testutil.TargetConfig,
		}

		listResult, err := instanceProv.List(ctx, listReq)
		require.NoError(t, err, "List should not return error")
		require.NotNil(t, listResult, "List result should not be nil")

		t.Logf("Found %d Bigtable instances in project %s", len(listResult.NativeIDs), testutil.Project)

		// Log and validate each instance
		for i, id := range listResult.NativeIDs {
			t.Logf("Instance %d: %s", i+1, id)

			// Verify native ID format: projects/{project}/instances/{instance}
			assert.Contains(t, id, "projects/", "Native ID should contain 'projects/'")
			assert.Contains(t, id, "/instances/", "Native ID should contain '/instances/'")

			// Read to get properties
			readReq := &resource.ReadRequest{
				NativeID:     id,
				TargetConfig: testutil.TargetConfig,
			}
			readResult, err := instanceProv.Read(ctx, readReq)
			require.NoError(t, err, "Read should not return error")
			require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

			var props map[string]interface{}
			err = json.Unmarshal([]byte(readResult.Properties), &props)
			require.NoError(t, err, "Properties should be valid JSON")

			// Log key properties if available
			if displayName, ok := props["displayName"].(string); ok {
				t.Logf("  Display Name: %s", displayName)
			}
			if instanceType, ok := props["type"].(string); ok {
				t.Logf("  Type: %s", instanceType)
			}
			if state, ok := props["state"].(string); ok {
				t.Logf("  State: %s", state)
			}
		}
	})

	t.Run("ListInstancesWithPagination", func(t *testing.T) {
		// Test with small page size to verify pagination works
		listReq := &resource.ListRequest{
			ResourceType: InstanceResourceType,
			TargetConfig: testutil.TargetConfig,
			PageSize:     2, // Request small page size
		}

		listResult, err := instanceProv.List(ctx, listReq)
		require.NoError(t, err, "List with pagination should not return error")
		require.NotNil(t, listResult, "List result should not be nil")

		t.Logf("First page: %d instances", len(listResult.NativeIDs))

		// If there are more instances, verify pagination token is returned
		if listResult.NextPageToken != nil && *listResult.NextPageToken != "" {
			t.Logf("Next page token available: %s...", (*listResult.NextPageToken)[:min(20, len(*listResult.NextPageToken))])

			// Fetch next page
			listReq.PageToken = listResult.NextPageToken
			nextResult, err := instanceProv.List(ctx, listReq)
			require.NoError(t, err, "List next page should not return error")
			require.NotNil(t, nextResult, "Next page result should not be nil")

			t.Logf("Second page: %d instances", len(nextResult.NativeIDs))
		} else {
			t.Logf("All instances fit in single page (no pagination token)")
		}
	})

	t.Run("ListInstancesVerifyProperties", func(t *testing.T) {
		listReq := &resource.ListRequest{
			ResourceType: InstanceResourceType,
			TargetConfig: testutil.TargetConfig,
		}

		listResult, err := instanceProv.List(ctx, listReq)
		require.NoError(t, err, "List should not return error")

		// Skip if no instances exist
		if len(listResult.NativeIDs) == 0 {
			t.Skip("No Bigtable instances found in project - skipping property verification")
		}

		// Verify properties of each instance
		for _, id := range listResult.NativeIDs {
			// Read to get properties
			readReq := &resource.ReadRequest{
				NativeID:     id,
				TargetConfig: testutil.TargetConfig,
			}
			readResult, err := instanceProv.Read(ctx, readReq)
			require.NoError(t, err, "Read should not return error")

			var props map[string]interface{}
			err = json.Unmarshal([]byte(readResult.Properties), &props)
			require.NoError(t, err, "Properties should be valid JSON for instance %s", id)

			// Verify expected fields are present
			assert.NotNil(t, props["name"], "Instance should have name field")
			assert.NotNil(t, props["state"], "Instance should have state field")
			assert.NotNil(t, props["type"], "Instance should have type field")

			// Verify state is valid
			state, ok := props["state"].(string)
			if ok {
				validStates := []string{"STATE_NOT_KNOWN", "READY", "CREATING"}
				assert.Contains(t, validStates, state, "State should be a valid Bigtable instance state")
			}

			// Verify type is valid
			instanceType, ok := props["type"].(string)
			if ok {
				validTypes := []string{"TYPE_UNSPECIFIED", "PRODUCTION", "DEVELOPMENT"}
				assert.Contains(t, validTypes, instanceType, "Type should be a valid Bigtable instance type")
			}
		}
	})
}
