// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration
// +build integration

package sql

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/compute"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/testutil"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupServiceNetworking sets up Service Networking for Cloud SQL private IP
// This only needs to be done once per project, but it's safe to call multiple times
//
// Prerequisites:
//  1. Enable Service Networking API:
//     gcloud services enable servicenetworking.googleapis.com --project=PROJECT_ID
//  2. Or visit: https://console.developers.google.com/apis/api/servicenetworking.googleapis.com/overview?project=PROJECT_ID
func setupServiceNetworking(t *testing.T, ctx context.Context) error {
	t.Helper()

	helper := compute.NewServiceNetworkingHelper(testutil.Config)

	// Set up Service Networking with a reserved IP range
	network := fmt.Sprintf("projects/%s/global/networks/default", testutil.Project)
	addressName := "google-managed-services-default"
	prefixLength := 16

	t.Logf("Setting up Service Networking for project %s (this may take a few minutes if not already configured)...", testutil.Project)

	err := helper.SetupServiceNetworkingForSQL(ctx, network, addressName, prefixLength)
	if err != nil {
		if strings.Contains(err.Error(), "SERVICE_DISABLED") || strings.Contains(err.Error(), "API has not been used") {
			return fmt.Errorf("Service Networking API is not enabled. Enable it with:\n"+
				"  gcloud services enable servicenetworking.googleapis.com --project=%s\n"+
				"Original error: %w", testutil.Project, err)
		}
		return fmt.Errorf("failed to setup service networking: %w", err)
	}

	t.Logf("Service Networking is ready")
	return nil
}

// TestDatabaseInstanceCreate tests the full CRUD lifecycle of a Cloud SQL Database Instance
func TestDatabaseInstanceCreate(t *testing.T) {
	ctx := context.Background()

	// Set up Service Networking (required for private IP)
	err := setupServiceNetworking(t, ctx)
	require.NoError(t, err, "Failed to setup Service Networking")

	// Create provisioner instance
	instance, err := NewSQLProvisioner(testutil.Config, DatabaseInstanceResourceType)
	require.NoError(t, err, "Failed to create SQLProvisioner")

	instanceName := fmt.Sprintf("formae-test-sql-%s", uuid.New().String()[:8])
	t.Logf("Creating test database instance: %s", instanceName)

	// Poll configuration for long-running SQL operations (5-10 minutes)
	pollConfig := testutil.NewPollConfig().
		WithMaxAttempts(150).                // 15 minutes with 6s intervals
		WithCheckInterval(6 * time.Second).
		WithResourceType(DatabaseInstanceResourceType).
		ForCreate().
		Build()

	// Test Create operation
	t.Run("Create", func(t *testing.T) {
		instanceProperties := map[string]interface{}{
			"name":            instanceName,
			"databaseVersion": "MYSQL_8_0",
			"region":          testutil.Region,
			"settings": map[string]interface{}{
				"tier":             "db-f1-micro",
				"availabilityType": "ZONAL",
				"dataDiskSizeGb":   10,
				"dataDiskType":     "PD_SSD",
				"backupConfiguration": map[string]interface{}{
					"enabled":   true,
					"startTime": "03:00",
				},
				"ipConfiguration": map[string]interface{}{
					// Disable public IP to comply with org policy constraints/sql.restrictPublicIp
					"ipv4Enabled": false,
					// Enable private IP using the default VPC
					// NOTE: Requires Service Networking to be configured once per project:
					//   1. Reserve an IP range: gcloud compute addresses create google-managed-services-default \
					//      --global --purpose=VPC_PEERING --prefix-length=16 --network=default --project=PROJECT_ID
					//   2. Create peering: gcloud services vpc-peerings connect \
					//      --service=servicenetworking.googleapis.com --ranges=google-managed-services-default \
					//      --network=default --project=PROJECT_ID
					"privateNetwork": fmt.Sprintf("projects/%s/global/networks/default", testutil.Project),
				},
			},
		}

		instancePropsJSON, err := json.Marshal(instanceProperties)
		require.NoError(t, err, "Failed to marshal instance properties")

		createReq := &resource.CreateRequest{
			ResourceType: DatabaseInstanceResourceType,
			Properties:   instancePropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		// Create the database instance
		createResult, err := instance.Create(ctx, createReq)
		require.NoError(t, err, "Create operation should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

		// Log any failure messages for debugging
		if createResult.ProgressResult.OperationStatus == resource.OperationStatusFailure {
			t.Logf("Create failed: %s (code: %s)", createResult.ProgressResult.StatusMessage, createResult.ProgressResult.ErrorCode)
		}

		assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation, "Operation should be Create")
		assert.Equal(t, resource.OperationStatusInProgress, createResult.ProgressResult.OperationStatus, "Should be in progress")
		require.NotEmpty(t, createResult.ProgressResult.RequestID, "RequestID should be set")
		require.NotEmpty(t, createResult.ProgressResult.NativeID, "NativeID should be set")

		nativeID := createResult.ProgressResult.NativeID
		t.Logf("Database instance creation initiated with RequestID: %s, NativeID: %s",
			createResult.ProgressResult.RequestID, nativeID)

		// Wait for creation to complete
		statusResult, err := testutil.WaitForCreateWithConfig(t, ctx, instance, createResult, testutil.TargetConfig, DatabaseInstanceResourceType, pollConfig)
		require.NoError(t, err, "Wait for create should not return error")
		require.NotNil(t, statusResult, "Status result should not be nil")
		require.Equal(t, resource.OperationStatusSuccess, statusResult.ProgressResult.OperationStatus, "Operation should succeed")

		t.Logf("Database instance created successfully")

		// Test Read operation
		t.Run("Read", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: DatabaseInstanceResourceType,
			}

			readResult, err := instance.Read(ctx, readReq)
			require.NoError(t, err, "Read operation should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			require.Empty(t, readResult.ErrorCode, "Read should not have error code")
			require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

			// Verify properties
			readProps, err := utils.ParseProperties(readResult.Properties)
			require.NoError(t, err, "Failed to parse read properties")

			assert.Equal(t, instanceName, utils.GetString(readProps, "name"), "Instance name should match")
			assert.Equal(t, "MYSQL_8_0", utils.GetString(readProps, "databaseVersion"), "Database version should match")
			assert.Equal(t, testutil.Region, utils.GetString(readProps, "region"), "Region should match")
			assert.Contains(t, []string{"RUNNABLE", "CREATING"}, utils.GetString(readProps, "state"), "State should be RUNNABLE or CREATING")

			// Verify settings
			settings := utils.GetObject(readProps, "settings")
			require.NotNil(t, settings, "Settings should exist")
			assert.Equal(t, "db-f1-micro", utils.GetString(settings, "tier"), "Tier should match")

			t.Logf("Read database instance properties successfully")
		})

		// Test List operation
		t.Run("List", func(t *testing.T) {
			listReq := &resource.ListRequest{
				ResourceType: DatabaseInstanceResourceType,
				TargetConfig: testutil.TargetConfig,
			}

			listResult, err := instance.List(ctx, listReq)
			require.NoError(t, err, "List operation should not return error")
			require.NotNil(t, listResult, "List result should not be nil")
			require.NotNil(t, listResult.NativeIDs, "NativeIDs list should not be nil")

			t.Logf("Found %d database instances in project", len(listResult.NativeIDs))

			// Verify our instance is in the list by reading each and checking the name
			found := false
			for _, id := range listResult.NativeIDs {
				readReq := &resource.ReadRequest{
					NativeID:     id,
					TargetConfig: testutil.TargetConfig,
					ResourceType: DatabaseInstanceResourceType,
				}
				readResult, err := instance.Read(ctx, readReq)
				if err != nil || readResult.ErrorCode != "" {
					continue
				}
				var props map[string]interface{}
				err = json.Unmarshal([]byte(readResult.Properties), &props)
				if err == nil && utils.GetString(props, "name") == instanceName {
					found = true
					break
				}
			}
			assert.True(t, found, "Created instance should be in the list")
		})

		// Test Delete operation
		t.Run("Delete", func(t *testing.T) {
			deleteReq := &resource.DeleteRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: DatabaseInstanceResourceType,
			}

			deleteResult, err := instance.Delete(ctx, deleteReq)
			require.NoError(t, err, "Delete operation should not return error")
			require.NotNil(t, deleteResult, "Delete result should not be nil")
			require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")

			assert.Equal(t, resource.OperationDelete, deleteResult.ProgressResult.Operation, "Operation should be Delete")
			assert.Equal(t, resource.OperationStatusInProgress, deleteResult.ProgressResult.OperationStatus, "Should be in progress")
			require.NotEmpty(t, deleteResult.ProgressResult.RequestID, "RequestID should be set")

			t.Logf("Database instance deletion initiated with RequestID: %s", deleteResult.ProgressResult.RequestID)

			// Wait for deletion to complete
			deletePollConfig := pollConfig
			deletePollConfig.OperationName = "Delete"
			_, err = testutil.WaitForDeleteWithConfig(t, ctx, instance, deleteResult, testutil.TargetConfig, DatabaseInstanceResourceType, deletePollConfig)
			require.NoError(t, err, "Wait for delete should not return error")

			t.Logf("Database instance deleted successfully")
		})

		// Verify deletion
		t.Run("VerifyDeleted", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: DatabaseInstanceResourceType,
			}

			readResult, err := instance.Read(ctx, readReq)
			require.NoError(t, err, "Read should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode, "Should return NotFound")

			t.Logf("Verified database instance was deleted")
		})
	})
}

// TestDatabaseInstanceNotFound tests reading a non-existent database instance
func TestDatabaseInstanceNotFound(t *testing.T) {
	instance, err := NewSQLProvisioner(testutil.Config, DatabaseInstanceResourceType)
	require.NoError(t, err)

	readReq := &resource.ReadRequest{
		NativeID:     fmt.Sprintf("projects/%s/instances/nonexistent-instance", testutil.Project),
		TargetConfig: testutil.TargetConfig,
		ResourceType: DatabaseInstanceResourceType,
	}

	readResult, err := instance.Read(context.Background(), readReq)
	require.NoError(t, err, "Read should not return error")
	require.NotNil(t, readResult, "Read result should not be nil")
	assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode, "Should return NotFound")

	t.Logf("Verified database instance not found")
}
