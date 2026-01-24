// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration
// +build integration

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

// TestNetworkCreate tests the creation, reading, and deletion of a GCP Network
func TestNetworkCreate(t *testing.T) {
	// Create provisioner instance
	network, err := NewComputeProvisioner(testutil.Config, NetworkResourceType)
	require.NoError(t, err, "Failed to create network provisioner")

	// Generate unique network name
	networkName := fmt.Sprintf("formae-test-network-%s", uuid.New().String()[:8])
	t.Logf("Creating test network: %s", networkName)

	ctx := context.Background()

	// Prepare network properties
	properties := map[string]interface{}{
		"name":                  networkName,
		"description":           "Test network created by Formae integration test",
		"autoCreateSubnetworks": false,
		"routingConfig": map[string]interface{}{
			"routingMode": "REGIONAL",
		},
		"mtu": 1460,
	}

	propertiesJSON, err := json.Marshal(properties)
	require.NoError(t, err, "Failed to marshal properties")

	// Create request
	createReq := &resource.CreateRequest{
		ResourceType: "GCP::Compute::Network",
		Properties:   propertiesJSON,
		TargetConfig: testutil.TargetConfig,
	}

	var nativeID string

	// Test Create operation
	t.Run("Create", func(t *testing.T) {
		createResult, err := network.Create(ctx, createReq)
		require.NoError(t, err, "Create operation should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation, "Operation should be Create")
		assert.Equal(t, resource.OperationStatusInProgress, createResult.ProgressResult.OperationStatus, "Should be in progress")
		require.NotEmpty(t, createResult.ProgressResult.RequestID, "RequestID should be set")

		t.Logf("Network creation initiated with RequestID: %s", createResult.ProgressResult.RequestID)

		// Wait for creation to complete
		statusResult, err := testutil.WaitForCreate(t, ctx, network, createResult, testutil.TargetConfig, "GCP::Compute::Network")
		require.NoError(t, err, "Network creation should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")

		nativeID = statusResult.ProgressResult.NativeID
		t.Logf("Network created with native ID: %s", nativeID)

		// Verify native ID format
		expectedNativeID := fmt.Sprintf("projects/%s/global/networks/%s", testutil.Project, networkName)
		assert.Equal(t, expectedNativeID, nativeID, "Native ID should match expected format")
	})

	// Test Read operation
	t.Run("Read", func(t *testing.T) {
		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Compute::Network",
		}

		readResult, err := network.Read(ctx, readReq)
		require.NoError(t, err, "Read operation should not return error")
		require.NotNil(t, readResult, "Read result should not be nil")
		require.Empty(t, readResult.ErrorCode, "Read should not have error code")
		require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

		// Verify properties
		var readProps map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &readProps)
		require.NoError(t, err, "Failed to unmarshal read properties")

		assert.Equal(t, networkName, readProps["name"], "Network name should match")
		assert.Equal(t, "Test network created by Formae integration test", readProps["description"], "Description should match")
		t.Logf("Read network properties successfully")
	})

	// Test List operation
	t.Run("List", func(t *testing.T) {
		listReq := &resource.ListRequest{
			ResourceType: "GCP::Compute::Network",
			TargetConfig: testutil.TargetConfig,
		}

		listResult, err := network.List(ctx, listReq)
		require.NoError(t, err, "List operation should not return error")
		require.NotNil(t, listResult, "List result should not be nil")
		assert.NotEmpty(t, listResult.NativeIDs, "Resources list should not be empty")

		// Debug: log all native IDs
		t.Logf("Looking for native ID: %s", nativeID)
		t.Logf("Found %d networks:", len(listResult.NativeIDs))
		for i, id := range listResult.NativeIDs {
			t.Logf("  [%d] NativeID: %s", i, id)
		}

		// Check if our test network is in the list
		found := false
		for _, id := range listResult.NativeIDs {
			if id == nativeID {
				found = true
				break
			}
		}
		assert.True(t, found, "Test network should be in the list")
		if found {
			t.Logf("Successfully found test network in the list")
		}
	})

	// Test Delete operation
	t.Run("Delete", func(t *testing.T) {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		deleteResult, err := network.Delete(ctx, deleteReq)
		require.NoError(t, err, "Delete operation should not return error")
		require.NotNil(t, deleteResult, "Delete result should not be nil")
		require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationDelete, deleteResult.ProgressResult.Operation, "Operation should be Delete")

		// Wait for deletion to complete
		statusResult, err := testutil.WaitForDelete(t, ctx, network, deleteResult, testutil.TargetConfig, "GCP::Compute::Network")
		require.NoError(t, err, "Network deletion should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")

		t.Logf("Network deleted successfully")
	})

	// Verify deletion by attempting to read (should fail)
	t.Run("VerifyDeleted", func(t *testing.T) {
		// Wait a bit to ensure deletion propagates
		time.Sleep(2 * time.Second)

		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Compute::Network",
		}

		readResult, err := network.Read(ctx, readReq)

		// Should return an error or NotFound status
		if err == nil {
			require.NotNil(t, readResult, "Read result should not be nil")
			assert.NotEmpty(t, readResult.ErrorCode, "Should have error code for deleted resource")
			assert.Contains(t, []resource.OperationErrorCode{
				resource.OperationErrorCodeNotFound,
				resource.OperationErrorCodeUnforeseenError,
			}, readResult.ErrorCode, "Error code should indicate resource not found")
		}

		t.Logf("Verified network was deleted")
	})
}
