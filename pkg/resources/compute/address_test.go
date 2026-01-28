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

	"github.com/google/uuid"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/testutil"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddressCreate tests the full CRUD lifecycle of a GCP Compute Address
func TestAddressCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create provisioner instance
	address, err := NewComputeProvisioner(testutil.Config, AddressResourceType)
	require.NoError(t, err, "Failed to create address provisioner")

	// Generate unique address name
	addressName := fmt.Sprintf("formae-test-addr-%s", uuid.New().String()[:8])
	t.Logf("Creating test address: %s", addressName)

	ctx := context.Background()

	// Test Create operation
	t.Run("Create", func(t *testing.T) {
		// Prepare address properties
		properties := map[string]interface{}{
			"name":        addressName,
			"description": "Test address created by Formae integration test",
			"addressType": "EXTERNAL",
			"networkTier": "PREMIUM",
			"labels": map[string]interface{}{
				"environment": "test",
				"managed-by":  "formae",
			},
		}

		propertiesJSON, err := json.Marshal(properties)
		require.NoError(t, err, "Failed to marshal properties")

		// Create request
		createReq := &resource.CreateRequest{
			ResourceType: AddressResourceType,
			Properties:   propertiesJSON,
			TargetConfig: testutil.TargetConfig,
		}

		// Create the address
		createResult, err := address.Create(ctx, createReq)
		require.NoError(t, err, "Create operation should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation, "Operation should be Create")
		assert.Equal(t, resource.OperationStatusInProgress, createResult.ProgressResult.OperationStatus, "Should be in progress")
		require.NotEmpty(t, createResult.ProgressResult.RequestID, "RequestID should be set")
		require.NotEmpty(t, createResult.ProgressResult.NativeID, "NativeID should be set")

		nativeID := createResult.ProgressResult.NativeID
		t.Logf("Address creation initiated with RequestID: %s, NativeID: %s",
			createResult.ProgressResult.RequestID, nativeID)

		// Wait for creation to complete
		statusResult, err := testutil.WaitForCreate(t, ctx, address, createResult, testutil.TargetConfig, AddressResourceType)
		require.NoError(t, err, "Wait for create should not return error")
		require.NotNil(t, statusResult, "Status result should not be nil")
		require.Equal(t, resource.OperationStatusSuccess, statusResult.ProgressResult.OperationStatus, "Operation should succeed")

		t.Logf("Address created successfully")

		// Test Read operation
		t.Run("Read", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: AddressResourceType,
			}

			readResult, err := address.Read(ctx, readReq)
			require.NoError(t, err, "Read operation should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			require.Empty(t, readResult.ErrorCode, "Read should not have error code")
			require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

			// Verify properties
			readProps, err := utils.ParseProperties(readResult.Properties)
			require.NoError(t, err, "Failed to parse read properties")

			assert.Equal(t, addressName, utils.GetString(readProps, "name"), "Address name should match")
			assert.Equal(t, "EXTERNAL", utils.GetString(readProps, "addressType"), "Address type should match")
			assert.NotEmpty(t, utils.GetString(readProps, "address"), "IP address should be assigned")

			// Verify labels
			if labels := utils.GetObject(readProps, "labels"); labels != nil {
				assert.Equal(t, "test", utils.GetString(labels, "environment"), "Environment label should match")
				assert.Equal(t, "formae", utils.GetString(labels, "managed-by"), "Managed-by label should match")
			}

			t.Logf("Read address properties successfully. IP: %s", utils.GetString(readProps, "address"))
		})

		// Note: Addresses are immutable after creation (except for labels which require a special setLabels endpoint)
		// Update operation is not supported via standard PATCH, so we skip the update test

		// Test List operation
		t.Run("List", func(t *testing.T) {
			listReq := &resource.ListRequest{
				ResourceType: AddressResourceType,
				TargetConfig: testutil.TargetConfig,
			}

			listResult, err := address.List(ctx, listReq)
			require.NoError(t, err, "List operation should not return error")
			require.NotNil(t, listResult, "List result should not be nil")
			require.NotNil(t, listResult.NativeIDs, "Resources list should not be nil")

			t.Logf("Found %d addresses in project", len(listResult.NativeIDs))

			// Verify our address is in the list
			found := false
			for _, id := range listResult.NativeIDs {
				if id == nativeID {
					found = true
					break
				}
			}
			assert.True(t, found, "Created address should be in the list")
		})

		// Test Delete operation
		t.Run("Delete", func(t *testing.T) {
			deleteReq := &resource.DeleteRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: AddressResourceType,
			}

			deleteResult, err := address.Delete(ctx, deleteReq)
			require.NoError(t, err, "Delete operation should not return error")
			require.NotNil(t, deleteResult, "Delete result should not be nil")
			require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")

			assert.Equal(t, resource.OperationDelete, deleteResult.ProgressResult.Operation, "Operation should be Delete")
			assert.Equal(t, resource.OperationStatusInProgress, deleteResult.ProgressResult.OperationStatus, "Should be in progress")
			require.NotEmpty(t, deleteResult.ProgressResult.RequestID, "RequestID should be set")

			t.Logf("Address deletion initiated with RequestID: %s", deleteResult.ProgressResult.RequestID)

			// Wait for deletion to complete
			_, err = testutil.WaitForDelete(t, ctx, address, deleteResult, testutil.TargetConfig, AddressResourceType)
			require.NoError(t, err, "Wait for delete should not return error")

			t.Logf("Address deleted successfully")
		})

		// Verify deletion
		t.Run("VerifyDeleted", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: AddressResourceType,
			}

			readResult, err := address.Read(ctx, readReq)
			require.NoError(t, err, "Read should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode, "Should return NotFound")

			t.Logf("Verified address was deleted")
		})
	})
}

// TestAddressNotFound tests reading a non-existent address
func TestAddressNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	address, err := NewComputeProvisioner(testutil.Config, AddressResourceType)
	require.NoError(t, err)

	readReq := &resource.ReadRequest{
		NativeID:     fmt.Sprintf("projects/%s/regions/%s/addresses/nonexistent-address", testutil.Project, testutil.Region),
		TargetConfig: testutil.TargetConfig,
		ResourceType: AddressResourceType,
	}

	readResult, err := address.Read(context.Background(), readReq)
	require.NoError(t, err, "Read should not return error")
	require.NotNil(t, readResult, "Read result should not be nil")
	assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode, "Should return NotFound")

	t.Logf("Verified address not found")
}

// TestAddressInternalType tests creating an internal address
func TestAddressInternalType(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	address, err := NewComputeProvisioner(testutil.Config, AddressResourceType)
	require.NoError(t, err, "Failed to create address provisioner")

	addressName := fmt.Sprintf("formae-test-internal-%s", uuid.New().String()[:8])
	t.Logf("Creating test internal address: %s", addressName)

	ctx := context.Background()

	// Prepare internal address properties
	properties := map[string]interface{}{
		"name":        addressName,
		"description": "Test internal address",
		"addressType": "INTERNAL",
		"purpose":     "GCE_ENDPOINT",
		"subnetwork":  fmt.Sprintf("projects/%s/regions/%s/subnetworks/default", testutil.Project, testutil.Region),
	}

	propertiesJSON, err := json.Marshal(properties)
	require.NoError(t, err, "Failed to marshal properties")

	createReq := &resource.CreateRequest{
		ResourceType: AddressResourceType,
		Properties:   propertiesJSON,
		TargetConfig: testutil.TargetConfig,
	}

	// Create the internal address
	createResult, err := address.Create(ctx, createReq)
	require.NoError(t, err, "Create operation should not return error")
	require.NotNil(t, createResult, "Create result should not be nil")

	// Wait for creation
	statusResult, err := testutil.WaitForCreate(t, ctx, address, createResult, testutil.TargetConfig, AddressResourceType)
	require.NoError(t, err, "Wait for create should not return error")
	require.Equal(t, resource.OperationStatusSuccess, statusResult.ProgressResult.OperationStatus, "Operation should succeed")

	nativeID := statusResult.ProgressResult.NativeID
	t.Logf("Internal address created successfully")

	// Read and verify
	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
		ResourceType: AddressResourceType,
	}

	readResult, err := address.Read(ctx, readReq)
	require.NoError(t, err, "Read operation should not return error")

	readProps, err := utils.ParseProperties(readResult.Properties)
	require.NoError(t, err, "Failed to parse read properties")

	assert.Equal(t, "INTERNAL", utils.GetString(readProps, "addressType"), "Address type should be INTERNAL")
	assert.Equal(t, "GCE_ENDPOINT", utils.GetString(readProps, "purpose"), "Purpose should match")
	t.Logf("Internal address IP: %s", utils.GetString(readProps, "address"))

	// Cleanup
	deleteReq := &resource.DeleteRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
		ResourceType: AddressResourceType,
	}

	deleteResult, err := address.Delete(ctx, deleteReq)
	require.NoError(t, err, "Delete operation should not return error")

	_, err = testutil.WaitForDelete(t, ctx, address, deleteResult, testutil.TargetConfig, AddressResourceType)
	require.NoError(t, err, "Wait for delete should not return error")

	t.Logf("Internal address deleted successfully")
}
