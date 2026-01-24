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

// TestClusterCreate tests the full CRUD lifecycle of a Bigtable cluster
func TestClusterCreate(t *testing.T) {
	ctx := context.Background()

	// Create provisioners
	instanceProv, err := NewBigtableProvisioner(testutil.Config, InstanceResourceType)
	require.NoError(t, err, "Failed to create Bigtable instance provisioner")

	clusterProv, err := NewBigtableProvisioner(testutil.Config, ClusterResourceType)
	require.NoError(t, err, "Failed to create Bigtable cluster provisioner")

	// Generate unique names
	instanceName := fmt.Sprintf("formae-test-bt-%s", strings.ToLower(uuid.New().String()[:8]))
	clusterName := fmt.Sprintf("cluster-%s", strings.ToLower(uuid.New().String()[:6]))

	var instanceNativeID string
	var clusterNativeID string

	// Test 1: Create Instance (prerequisite for cluster)
	t.Run("CreateInstance", func(t *testing.T) {
		instanceProperties := map[string]interface{}{
			"name":        instanceName,
			"displayName": "Formae Cluster Test",
			"type":        "PRODUCTION", // PRODUCTION instances require clusters
			"labels": map[string]interface{}{
				"test": "formae-bigtable-cluster",
			},
			// PRODUCTION instances require at least one cluster
			"clusters": map[string]interface{}{
				"initial-cluster": map[string]interface{}{
					"location":           testutil.Region + "-a", // Use zone in same region
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
			statusResult, err := testutil.PollUntilComplete(t, ctx, instanceProv,
				createResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
					MaxAttempts:   60,
					CheckInterval: 10 * time.Second,
					ResourceType:  InstanceResourceType,
					OperationName: "Create",
				})
			require.NoError(t, err)
			require.Equal(t, resource.OperationStatusSuccess, statusResult.ProgressResult.OperationStatus)
			instanceNativeID = statusResult.ProgressResult.NativeID
		} else {
			t.Logf("Create result status: %s, message: %s", createResult.ProgressResult.OperationStatus, createResult.ProgressResult.StatusMessage)
			require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)
			instanceNativeID = createResult.ProgressResult.NativeID
		}

		require.NotEmpty(t, instanceNativeID)
		t.Logf("Instance created: %s", instanceNativeID)
	})

	// Cleanup function for instance
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

	// Test 2: Create Cluster
	t.Run("CreateCluster", func(t *testing.T) {
		require.NotEmpty(t, instanceNativeID, "Instance ID should be set")

		clusterProperties := map[string]interface{}{
			"instance":           instanceName,
			"name":               clusterName,
			"location":           testutil.Region + "-b", // Use zone in same region
			"serveNodes":         1,
			"defaultStorageType": "SSD",
		}

		propsJSON, err := json.Marshal(clusterProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: ClusterResourceType,
			Properties:   propsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := clusterProv.Create(ctx, createReq)
		require.NoError(t, err)
		require.NotNil(t, createResult)
		require.NotNil(t, createResult.ProgressResult)

		// Cluster creation is async (LRO)
		if createResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
			require.NotEmpty(t, createResult.ProgressResult.RequestID)
			require.NotEmpty(t, createResult.ProgressResult.NativeID)

			t.Logf("Cluster creation initiated: %s", createResult.ProgressResult.RequestID)

			statusResult, err := testutil.PollUntilComplete(t, ctx, clusterProv,
				createResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
					MaxAttempts:   60,
					CheckInterval: 10 * time.Second,
					ResourceType:  ClusterResourceType,
					OperationName: "Create",
				})
			require.NoError(t, err)
			require.Equal(t, resource.OperationStatusSuccess, statusResult.ProgressResult.OperationStatus)
			clusterNativeID = statusResult.ProgressResult.NativeID
		} else {
			require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)
			clusterNativeID = createResult.ProgressResult.NativeID
		}

		require.NotEmpty(t, clusterNativeID)
		expectedID := fmt.Sprintf("projects/%s/instances/%s/clusters/%s", testutil.Project, instanceName, clusterName)
		assert.Equal(t, expectedID, clusterNativeID)
		t.Logf("Cluster created: %s", clusterNativeID)
	})

	// Test 3: Read Cluster
	t.Run("ReadCluster", func(t *testing.T) {
		require.NotEmpty(t, clusterNativeID, "Cluster ID should be set")

		readReq := &resource.ReadRequest{
			NativeID: clusterNativeID,
			TargetConfig: testutil.TargetConfig,
		}

		readResult, err := clusterProv.Read(ctx, readReq)
		require.NoError(t, err)
		require.NotNil(t, readResult)
		require.NotEmpty(t, readResult.Properties)

		var props map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &props)
		require.NoError(t, err)

		// Verify key properties
		assert.Contains(t, props["name"].(string), clusterName)
		assert.Equal(t, fmt.Sprintf("projects/%s/locations/%s", testutil.Project, testutil.Region+"-b"), props["location"])
		assert.Equal(t, "READY", props["state"])
		assert.Equal(t, "SSD", props["defaultStorageType"])

		t.Logf("Cluster properties: %+v", props)
	})

	// Test 4: List Clusters
	t.Run("ListClusters", func(t *testing.T) {
		require.NotEmpty(t, instanceName, "Instance name should be set")

		listReq := &resource.ListRequest{
			ResourceType: ClusterResourceType,
			TargetConfig: testutil.TargetConfig,
			AdditionalProperties: map[string]string{
				"instance": instanceName,
			},
		}

		listResult, err := clusterProv.List(ctx, listReq)
		require.NoError(t, err)
		require.NotNil(t, listResult)
		require.NotEmpty(t, listResult.NativeIDs)

		// Verify our cluster is in the list
		found := false
		for _, id := range listResult.NativeIDs {
			if id == clusterNativeID {
				found = true
				break
			}
		}
		assert.True(t, found, "Our cluster should be in the list")
		t.Logf("Listed %d clusters in instance", len(listResult.NativeIDs))
	})

	// Test 5: Update Cluster (should return NotUpdatable)
	t.Run("UpdateCluster", func(t *testing.T) {
		require.NotEmpty(t, clusterNativeID)

		updatedProperties := map[string]interface{}{
			"instance":   instanceName,
			"name":       clusterName,
			"serveNodes": 2,
		}

		propsJSON, err := json.Marshal(updatedProperties)
		require.NoError(t, err)

		updateReq := &resource.UpdateRequest{
			NativeID:          clusterNativeID,
			ResourceType:      ClusterResourceType,
			DesiredProperties: propsJSON,
			TargetConfig:      testutil.TargetConfig,
		}

		updateResult, err := clusterProv.Update(ctx, updateReq)
		require.NoError(t, err)
		require.NotNil(t, updateResult)
		require.Equal(t, resource.OperationStatusFailure, updateResult.ProgressResult.OperationStatus)
		require.Equal(t, resource.OperationErrorCodeNotUpdatable, updateResult.ProgressResult.ErrorCode)

		t.Logf("Update correctly returned NotUpdatable")
	})

	// Test 6: Delete Cluster
	t.Run("DeleteCluster", func(t *testing.T) {
		require.NotEmpty(t, clusterNativeID)

		deleteReq := &resource.DeleteRequest{
			NativeID: clusterNativeID,
			TargetConfig: testutil.TargetConfig,
		}

		deleteResult, err := clusterProv.Delete(ctx, deleteReq)
		require.NoError(t, err)
		require.NotNil(t, deleteResult)

		if deleteResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
			statusResult, err := testutil.PollUntilComplete(t, ctx, clusterProv,
				deleteResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
					MaxAttempts:   60,
					CheckInterval: 10 * time.Second,
					ResourceType:  ClusterResourceType,
					OperationName: "Delete",
				})
			require.NoError(t, err)
			require.Equal(t, resource.OperationStatusSuccess, statusResult.ProgressResult.OperationStatus)
		} else {
			require.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus)
		}

		t.Logf("Cluster deleted: %s", clusterNativeID)
	})

	// Test 7: Verify Cluster is Deleted
	t.Run("VerifyClusterDeleted", func(t *testing.T) {
		require.NotEmpty(t, clusterNativeID)

		readReq := &resource.ReadRequest{
			NativeID: clusterNativeID,
			TargetConfig: testutil.TargetConfig,
		}

		readResult, err := clusterProv.Read(ctx, readReq)
		require.NoError(t, err)
		require.NotNil(t, readResult)
		require.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode)

		t.Logf("Verified cluster is deleted (not found)")
	})
}

// TestClusterReadNonExistent tests reading a non-existent cluster
func TestClusterReadNonExistent(t *testing.T) {
	ctx := context.Background()

	clusterProv, err := NewBigtableProvisioner(testutil.Config, ClusterResourceType)
	require.NoError(t, err)

	nonExistentID := fmt.Sprintf("projects/%s/instances/nonexistent/clusters/nonexistent-%s",
		testutil.Project,
		strings.ToLower(uuid.New().String()[:8]))

	readReq := &resource.ReadRequest{
		NativeID: nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := clusterProv.Read(ctx, readReq)
	require.NoError(t, err)
	require.NotNil(t, readResult)
	require.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode)

	t.Logf("Correctly handled read of non-existent cluster: %s", nonExistentID)
}

// TestClusterCreateMissingInstance tests creating a cluster without specifying instance
func TestClusterCreateMissingInstance(t *testing.T) {
	ctx := context.Background()

	clusterProv, err := NewBigtableProvisioner(testutil.Config, ClusterResourceType)
	require.NoError(t, err)

	clusterProperties := map[string]interface{}{
		"name":               "test-cluster",
		"location":           testutil.Region + "-a", // Use zone in same region
		"serveNodes":         1,
		"defaultStorageType": "SSD",
	}

	propsJSON, err := json.Marshal(clusterProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: ClusterResourceType,
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := clusterProv.Create(ctx, createReq)
	require.NoError(t, err)
	require.NotNil(t, createResult)
	require.Equal(t, resource.OperationStatusFailure, createResult.ProgressResult.OperationStatus)
	require.Equal(t, resource.OperationErrorCodeInvalidRequest, createResult.ProgressResult.ErrorCode)
	assert.Contains(t, createResult.ProgressResult.StatusMessage, "instance")

	t.Logf("Correctly rejected create with missing instance")
}
