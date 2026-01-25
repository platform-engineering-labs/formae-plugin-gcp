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

// createTestProductionInstance creates a PRODUCTION Bigtable instance for cluster tests.
// PRODUCTION instances are required to add additional clusters.
func createTestProductionInstance(t *testing.T, ctx context.Context) (string, string, func()) {
	t.Helper()

	instanceProv, err := NewBigtableProvisioner(testutil.Config, InstanceResourceType)
	require.NoError(t, err)

	instanceName := fmt.Sprintf("formae-test-bt-%s", strings.ToLower(uuid.New().String()[:8]))

	instanceProperties := map[string]interface{}{
		"name":        instanceName,
		"displayName": "Formae Cluster Test",
		"type":        "PRODUCTION",
		"labels": map[string]interface{}{
			"test": "formae-bigtable-cluster",
		},
		"clusters": map[string]interface{}{
			"initial-cluster": map[string]interface{}{
				"location":           testutil.Region + "-a",
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

	var nativeID string
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
		nativeID = statusResult.ProgressResult.NativeID
	} else {
		require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus,
			"Create failed: %s", createResult.ProgressResult.StatusMessage)
		nativeID = createResult.ProgressResult.NativeID
	}

	cleanup := func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		deleteResult, err := instanceProv.Delete(ctx, deleteReq)
		if err == nil && deleteResult != nil && deleteResult.ProgressResult != nil {
			if deleteResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress &&
				deleteResult.ProgressResult.RequestID != "" {
				_, _ = testutil.PollUntilComplete(t, ctx, instanceProv,
					deleteResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
						MaxAttempts:   60,
						CheckInterval: 10 * time.Second,
						ResourceType:  InstanceResourceType,
						OperationName: "Delete",
					})
			}
		}
	}

	return instanceName, nativeID, cleanup
}

// TestCluster_Create_Integration tests creating a Bigtable cluster.
func TestCluster_Create_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	instanceName, _, cleanupInstance := createTestProductionInstance(t, ctx)
	defer cleanupInstance()

	clusterProv, err := NewBigtableProvisioner(testutil.Config, ClusterResourceType)
	require.NoError(t, err)

	clusterName := fmt.Sprintf("cluster-%s", strings.ToLower(uuid.New().String()[:6]))

	clusterProperties := map[string]interface{}{
		"instance":           instanceName,
		"name":               clusterName,
		"location":           testutil.Region + "-b",
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

	var clusterNativeID string
	if createResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
		require.NotEmpty(t, createResult.ProgressResult.RequestID)
		require.NotEmpty(t, createResult.ProgressResult.NativeID)

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

	// Cleanup cluster
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     clusterNativeID,
			TargetConfig: testutil.TargetConfig,
		}
		deleteResult, _ := clusterProv.Delete(ctx, deleteReq)
		if deleteResult != nil && deleteResult.ProgressResult != nil &&
			deleteResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress &&
			deleteResult.ProgressResult.RequestID != "" {
			_, _ = testutil.PollUntilComplete(t, ctx, clusterProv,
				deleteResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
					MaxAttempts:   60,
					CheckInterval: 10 * time.Second,
					ResourceType:  ClusterResourceType,
					OperationName: "Delete",
				})
		}
	}()

	require.NotEmpty(t, clusterNativeID)
	expectedID := fmt.Sprintf("projects/%s/instances/%s/clusters/%s", testutil.Project, instanceName, clusterName)
	assert.Equal(t, expectedID, clusterNativeID)
	t.Logf("Cluster created: %s", clusterNativeID)
}

// TestCluster_Read_Integration tests reading a Bigtable cluster.
func TestCluster_Read_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	instanceName, _, cleanupInstance := createTestProductionInstance(t, ctx)
	defer cleanupInstance()

	clusterProv, err := NewBigtableProvisioner(testutil.Config, ClusterResourceType)
	require.NoError(t, err)

	clusterName := fmt.Sprintf("cluster-%s", strings.ToLower(uuid.New().String()[:6]))

	// Create cluster
	clusterProperties := map[string]interface{}{
		"instance":           instanceName,
		"name":               clusterName,
		"location":           testutil.Region + "-b",
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

	var clusterNativeID string
	if createResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
		statusResult, err := testutil.PollUntilComplete(t, ctx, clusterProv,
			createResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
				MaxAttempts:   60,
				CheckInterval: 10 * time.Second,
				ResourceType:  ClusterResourceType,
				OperationName: "Create",
			})
		require.NoError(t, err)
		clusterNativeID = statusResult.ProgressResult.NativeID
	} else {
		clusterNativeID = createResult.ProgressResult.NativeID
	}

	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     clusterNativeID,
			TargetConfig: testutil.TargetConfig,
		}
		deleteResult, _ := clusterProv.Delete(ctx, deleteReq)
		if deleteResult != nil && deleteResult.ProgressResult != nil &&
			deleteResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress &&
			deleteResult.ProgressResult.RequestID != "" {
			_, _ = testutil.PollUntilComplete(t, ctx, clusterProv,
				deleteResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
					MaxAttempts:   60,
					CheckInterval: 10 * time.Second,
					ResourceType:  ClusterResourceType,
					OperationName: "Delete",
				})
		}
	}()

	// Read cluster
	readReq := &resource.ReadRequest{
		NativeID:     clusterNativeID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := clusterProv.Read(ctx, readReq)
	require.NoError(t, err)
	require.NotNil(t, readResult)
	require.NotEmpty(t, readResult.Properties)

	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err)

	assert.Contains(t, props["name"].(string), clusterName)
	assert.Equal(t, fmt.Sprintf("projects/%s/locations/%s", testutil.Project, testutil.Region+"-b"), props["location"])
	assert.Equal(t, "READY", props["state"])
	assert.Equal(t, "SSD", props["defaultStorageType"])
	t.Logf("Cluster read successfully")
}

// TestCluster_Update_NotUpdatable_Integration tests that cluster updates return NotUpdatable.
func TestCluster_Update_NotUpdatable_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	instanceName, _, cleanupInstance := createTestProductionInstance(t, ctx)
	defer cleanupInstance()

	clusterProv, err := NewBigtableProvisioner(testutil.Config, ClusterResourceType)
	require.NoError(t, err)

	clusterName := fmt.Sprintf("cluster-%s", strings.ToLower(uuid.New().String()[:6]))

	// Create cluster
	clusterProperties := map[string]interface{}{
		"instance":           instanceName,
		"name":               clusterName,
		"location":           testutil.Region + "-b",
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

	var clusterNativeID string
	if createResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
		statusResult, err := testutil.PollUntilComplete(t, ctx, clusterProv,
			createResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
				MaxAttempts:   60,
				CheckInterval: 10 * time.Second,
				ResourceType:  ClusterResourceType,
				OperationName: "Create",
			})
		require.NoError(t, err)
		clusterNativeID = statusResult.ProgressResult.NativeID
	} else {
		clusterNativeID = createResult.ProgressResult.NativeID
	}

	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     clusterNativeID,
			TargetConfig: testutil.TargetConfig,
		}
		deleteResult, _ := clusterProv.Delete(ctx, deleteReq)
		if deleteResult != nil && deleteResult.ProgressResult != nil &&
			deleteResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress &&
			deleteResult.ProgressResult.RequestID != "" {
			_, _ = testutil.PollUntilComplete(t, ctx, clusterProv,
				deleteResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
					MaxAttempts:   60,
					CheckInterval: 10 * time.Second,
					ResourceType:  ClusterResourceType,
					OperationName: "Delete",
				})
		}
	}()

	// Attempt to update
	updatedProperties := map[string]interface{}{
		"instance":   instanceName,
		"name":       clusterName,
		"serveNodes": 2,
	}

	updatedPropsJSON, err := json.Marshal(updatedProperties)
	require.NoError(t, err)

	updateReq := &resource.UpdateRequest{
		NativeID:          clusterNativeID,
		ResourceType:      ClusterResourceType,
		DesiredProperties: updatedPropsJSON,
		TargetConfig:      testutil.TargetConfig,
	}

	updateResult, err := clusterProv.Update(ctx, updateReq)
	require.NoError(t, err)
	require.NotNil(t, updateResult)
	require.Equal(t, resource.OperationStatusFailure, updateResult.ProgressResult.OperationStatus)
	require.Equal(t, resource.OperationErrorCodeNotUpdatable, updateResult.ProgressResult.ErrorCode)
	t.Logf("Update correctly returned NotUpdatable")
}

// TestCluster_Delete_Integration tests deleting a Bigtable cluster.
func TestCluster_Delete_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	instanceName, _, cleanupInstance := createTestProductionInstance(t, ctx)
	defer cleanupInstance()

	clusterProv, err := NewBigtableProvisioner(testutil.Config, ClusterResourceType)
	require.NoError(t, err)

	clusterName := fmt.Sprintf("cluster-%s", strings.ToLower(uuid.New().String()[:6]))

	// Create cluster
	clusterProperties := map[string]interface{}{
		"instance":           instanceName,
		"name":               clusterName,
		"location":           testutil.Region + "-b",
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

	var clusterNativeID string
	if createResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
		statusResult, err := testutil.PollUntilComplete(t, ctx, clusterProv,
			createResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
				MaxAttempts:   60,
				CheckInterval: 10 * time.Second,
				ResourceType:  ClusterResourceType,
				OperationName: "Create",
			})
		require.NoError(t, err)
		clusterNativeID = statusResult.ProgressResult.NativeID
	} else {
		clusterNativeID = createResult.ProgressResult.NativeID
	}

	// Delete cluster
	deleteReq := &resource.DeleteRequest{
		NativeID:     clusterNativeID,
		TargetConfig: testutil.TargetConfig,
	}

	deleteResult, err := clusterProv.Delete(ctx, deleteReq)
	require.NoError(t, err)
	require.NotNil(t, deleteResult)

	if deleteResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress &&
		deleteResult.ProgressResult.RequestID != "" {
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

	// Verify it's deleted
	readReq := &resource.ReadRequest{
		NativeID:     clusterNativeID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := clusterProv.Read(ctx, readReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode)
	t.Logf("Verified cluster is deleted (not found)")
}

// TestCluster_Delete_NotFound_Integration tests deleting a non-existent cluster.
func TestCluster_Delete_NotFound_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	clusterProv, err := NewBigtableProvisioner(testutil.Config, ClusterResourceType)
	require.NoError(t, err)

	nonExistentID := fmt.Sprintf("projects/%s/instances/nonexistent/clusters/nonexistent-%s",
		testutil.Project, strings.ToLower(uuid.New().String()[:8]))

	deleteReq := &resource.DeleteRequest{
		NativeID:     nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	deleteResult, err := clusterProv.Delete(ctx, deleteReq)
	require.NoError(t, err)
	require.NotNil(t, deleteResult)
	// Bigtable returns Failure for non-existent resources
	require.Equal(t, resource.OperationStatusFailure, deleteResult.ProgressResult.OperationStatus)
	t.Logf("Delete non-existent cluster returned status: %s", deleteResult.ProgressResult.OperationStatus)
}

// TestCluster_List_Integration tests listing Bigtable clusters.
func TestCluster_List_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	instanceName, _, cleanupInstance := createTestProductionInstance(t, ctx)
	defer cleanupInstance()

	clusterProv, err := NewBigtableProvisioner(testutil.Config, ClusterResourceType)
	require.NoError(t, err)

	clusterName := fmt.Sprintf("cluster-%s", strings.ToLower(uuid.New().String()[:6]))

	// Create cluster
	clusterProperties := map[string]interface{}{
		"instance":           instanceName,
		"name":               clusterName,
		"location":           testutil.Region + "-b",
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

	var clusterNativeID string
	if createResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
		statusResult, err := testutil.PollUntilComplete(t, ctx, clusterProv,
			createResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
				MaxAttempts:   60,
				CheckInterval: 10 * time.Second,
				ResourceType:  ClusterResourceType,
				OperationName: "Create",
			})
		require.NoError(t, err)
		clusterNativeID = statusResult.ProgressResult.NativeID
	} else {
		clusterNativeID = createResult.ProgressResult.NativeID
	}

	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     clusterNativeID,
			TargetConfig: testutil.TargetConfig,
		}
		deleteResult, _ := clusterProv.Delete(ctx, deleteReq)
		if deleteResult != nil && deleteResult.ProgressResult != nil &&
			deleteResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress &&
			deleteResult.ProgressResult.RequestID != "" {
			_, _ = testutil.PollUntilComplete(t, ctx, clusterProv,
				deleteResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
					MaxAttempts:   60,
					CheckInterval: 10 * time.Second,
					ResourceType:  ClusterResourceType,
					OperationName: "Delete",
				})
		}
	}()

	// List clusters
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
	t.Logf("Listed %d clusters in instance", len(listResult.NativeIDs))

	// Verify our cluster is in the list
	found := false
	for _, id := range listResult.NativeIDs {
		if id == clusterNativeID {
			found = true
			break
		}
	}
	assert.True(t, found, "Created cluster should be in the list")
}

// TestCluster_Read_NotFound_Integration tests reading a non-existent cluster.
func TestCluster_Read_NotFound_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	clusterProv, err := NewBigtableProvisioner(testutil.Config, ClusterResourceType)
	require.NoError(t, err)

	nonExistentID := fmt.Sprintf("projects/%s/instances/nonexistent/clusters/nonexistent-%s",
		testutil.Project, strings.ToLower(uuid.New().String()[:8]))

	readReq := &resource.ReadRequest{
		NativeID:     nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := clusterProv.Read(ctx, readReq)
	require.NoError(t, err)
	require.NotNil(t, readResult)
	require.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode)
	t.Logf("Correctly handled read of non-existent cluster")
}

// TestCluster_CreateInvalid_Integration tests error handling for invalid create requests.
func TestCluster_CreateInvalid_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	clusterProv, err := NewBigtableProvisioner(testutil.Config, ClusterResourceType)
	require.NoError(t, err)

	// Test with missing instance
	clusterProperties := map[string]interface{}{
		"name":               "test-cluster",
		"location":           testutil.Region + "-a",
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
