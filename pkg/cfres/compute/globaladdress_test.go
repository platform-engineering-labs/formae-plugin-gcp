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
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGlobalAddressCreate tests the creation, reading, and deletion of a GCP Global Address
func TestGlobalAddressCreate(t *testing.T) {
	// Create provisioner instance
	globalAddress, err := NewComputeProvisioner(testutil.Config, GlobalAddressResourceType)
	require.NoError(t, err, "Failed to create global address provisioner")

	// Generate unique name
	addressName := fmt.Sprintf("formae-test-gip-%s", uuid.New().String()[:8])
	t.Logf("Creating test global address: %s", addressName)

	ctx := context.Background()

	// Prepare address properties for external IP
	properties := map[string]interface{}{
		"name":        addressName,
		"description": "Test global address created by Formae integration test",
		"addressType": "EXTERNAL",
		"ipVersion":   "IPV4",
	}

	propertiesJSON, err := json.Marshal(properties)
	require.NoError(t, err, "Failed to marshal properties")

	// Create request
	createReq := &resource.CreateRequest{
		ResourceType: GlobalAddressResourceType,
			Properties:   propertiesJSON,
		TargetConfig: testutil.TargetConfig,
	}

	var nativeID string

	// Test Create operation
	t.Run("Create", func(t *testing.T) {
		createResult, err := globalAddress.Create(ctx, createReq)
		require.NoError(t, err, "Create operation should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation, "Operation should be Create")
		assert.Equal(t, resource.OperationStatusInProgress, createResult.ProgressResult.OperationStatus, "Should be in progress")
		require.NotEmpty(t, createResult.ProgressResult.RequestID, "RequestID should be set")

		t.Logf("Global address creation initiated with RequestID: %s", createResult.ProgressResult.RequestID)

		// Wait for creation to complete
		statusResult, err := testutil.WaitForCreate(t, ctx, globalAddress, createResult, testutil.TargetConfig, GlobalAddressResourceType)
		require.NoError(t, err, "Global address creation should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")

		nativeID = statusResult.ProgressResult.NativeID
		t.Logf("Global address created with native ID: %s", nativeID)

		// Verify native ID format
		expectedNativeID := fmt.Sprintf("projects/%s/global/addresses/%s", testutil.Project, addressName)
		assert.Equal(t, expectedNativeID, nativeID, "Native ID should match expected format")
	})

	// Test Read operation
	t.Run("Read", func(t *testing.T) {
		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: GlobalAddressResourceType,
		}

		readResult, err := globalAddress.Read(ctx, readReq)
		require.NoError(t, err, "Read operation should not return error")
		require.NotNil(t, readResult, "Read result should not be nil")
		require.Empty(t, readResult.ErrorCode, "Read should not have error code")
		require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

		// Verify properties
		var readProps map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &readProps)
		require.NoError(t, err, "Failed to unmarshal read properties")

		assert.Equal(t, addressName, readProps["name"], "Address name should match")
		assert.Equal(t, "Test global address created by Formae integration test", readProps["description"], "Description should match")
		assert.NotEmpty(t, readProps["address"], "Should have an allocated IP address")
		t.Logf("Read global address properties: IP=%s", readProps["address"])
	})

	// Test List operation
	t.Run("List", func(t *testing.T) {
		listReq := &resource.ListRequest{
			ResourceType: GlobalAddressResourceType,
			TargetConfig: testutil.TargetConfig,
		}

		listResult, err := globalAddress.List(ctx, listReq)
		require.NoError(t, err, "List operation should not return error")
		require.NotNil(t, listResult, "List result should not be nil")
		assert.NotEmpty(t, listResult.NativeIDs, "Resources list should not be empty")

		// Check if our test address is in the list
		found := false
		for _, id := range listResult.NativeIDs {
			if id == nativeID {
				found = true
				break
			}
		}
		assert.True(t, found, "Test global address should be in the list")
		t.Logf("Successfully found test global address in the list")
	})

	// Test Delete operation
	t.Run("Delete", func(t *testing.T) {
		deleteReq := &resource.DeleteRequest{
			NativeID: nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		deleteResult, err := globalAddress.Delete(ctx, deleteReq)
		require.NoError(t, err, "Delete operation should not return error")
		require.NotNil(t, deleteResult, "Delete result should not be nil")
		require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationDelete, deleteResult.ProgressResult.Operation, "Operation should be Delete")

		// Wait for deletion to complete
		statusResult, err := testutil.WaitForDelete(t, ctx, globalAddress, deleteResult, testutil.TargetConfig, GlobalAddressResourceType)
		require.NoError(t, err, "Global address deletion should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")

		t.Logf("Global address deleted successfully")
	})

	// Verify deletion
	t.Run("VerifyDeleted", func(t *testing.T) {
		time.Sleep(2 * time.Second)

		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: GlobalAddressResourceType,
		}

		readResult, err := globalAddress.Read(ctx, readReq)

		if err == nil {
			require.NotNil(t, readResult, "Read result should not be nil")
			assert.NotEmpty(t, readResult.ErrorCode, "Should have error code for deleted resource")
		}

		t.Logf("Verified global address was deleted")
	})
}
