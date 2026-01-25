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

// createTestInstance creates a Bigtable DEVELOPMENT instance and returns the native ID and a cleanup function.
// DEVELOPMENT instances are faster to create and don't require serveNodes.
func createTestInstance(t *testing.T, ctx context.Context) (string, string, func()) {
	t.Helper()

	instanceProv, err := NewBigtableProvisioner(testutil.Config, InstanceResourceType)
	require.NoError(t, err)

	instanceName := fmt.Sprintf("formae-test-bt-%s", strings.ToLower(uuid.New().String()[:8]))

	instanceProperties := map[string]interface{}{
		"name":        instanceName,
		"displayName": "Formae Test Instance",
		"type":        "DEVELOPMENT",
		"labels": map[string]interface{}{
			"test":        "formae-bigtable",
			"environment": "integration-test",
		},
		"clusters": map[string]interface{}{
			"cluster1": map[string]interface{}{
				"location":           testutil.Region + "-a",
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
		require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)
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

// TestInstance_Create_Integration tests creating a Bigtable instance.
func TestInstance_Create_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	instanceProv, err := NewBigtableProvisioner(testutil.Config, InstanceResourceType)
	require.NoError(t, err)

	instanceName := fmt.Sprintf("formae-test-bt-%s", strings.ToLower(uuid.New().String()[:8]))

	instanceProperties := map[string]interface{}{
		"name":        instanceName,
		"displayName": "Formae Test Instance",
		"type":        "DEVELOPMENT",
		"labels": map[string]interface{}{
			"test":        "formae-bigtable-instance",
			"environment": "integration-test",
		},
		"clusters": map[string]interface{}{
			"cluster1": map[string]interface{}{
				"location":           testutil.Region + "-a",
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
	require.NotNil(t, createResult.ProgressResult)

	var nativeID string
	if createResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
		require.NotEmpty(t, createResult.ProgressResult.RequestID)
		require.NotEmpty(t, createResult.ProgressResult.NativeID)
		nativeID = createResult.ProgressResult.NativeID

		statusResult, err := testutil.PollUntilComplete(t, ctx, instanceProv,
			createResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
				MaxAttempts:   60,
				CheckInterval: 10 * time.Second,
				ResourceType:  InstanceResourceType,
				OperationName: "Create",
			})
		require.NoError(t, err)
		require.Equal(t, resource.OperationStatusSuccess, statusResult.ProgressResult.OperationStatus)
		t.Logf("Instance created: %s", nativeID)
	} else {
		require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)
		nativeID = createResult.ProgressResult.NativeID
		t.Logf("Instance created synchronously: %s", nativeID)
	}

	// Cleanup
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		deleteResult, _ := instanceProv.Delete(ctx, deleteReq)
		if deleteResult != nil && deleteResult.ProgressResult != nil &&
			deleteResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress &&
			deleteResult.ProgressResult.RequestID != "" {
			_, _ = testutil.PollUntilComplete(t, ctx, instanceProv,
				deleteResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
					MaxAttempts:   60,
					CheckInterval: 10 * time.Second,
					ResourceType:  InstanceResourceType,
					OperationName: "Delete",
				})
		}
	}()

	// Verify native ID format
	expectedPrefix := fmt.Sprintf("projects/%s/instances/%s", testutil.Project, instanceName)
	assert.Equal(t, expectedPrefix, nativeID)
}

// TestInstance_Read_Integration tests reading a Bigtable instance.
func TestInstance_Read_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	_, nativeID, cleanup := createTestInstance(t, ctx)
	defer cleanup()

	instanceProv, err := NewBigtableProvisioner(testutil.Config, InstanceResourceType)
	require.NoError(t, err)

	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := instanceProv.Read(ctx, readReq)
	require.NoError(t, err)
	require.NotNil(t, readResult)
	require.NotEmpty(t, readResult.Properties)

	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err)

	assert.Equal(t, "Formae Test Instance", props["displayName"])
	assert.Equal(t, "DEVELOPMENT", props["type"])
	assert.Equal(t, "READY", props["state"])

	if labels, ok := props["labels"].(map[string]interface{}); ok {
		assert.Equal(t, "formae-bigtable", labels["test"])
		assert.Equal(t, "integration-test", labels["environment"])
	}
	t.Logf("Instance read successfully")
}

// TestInstance_Update_NotUpdatable_Integration tests that instance updates return NotUpdatable.
func TestInstance_Update_NotUpdatable_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	instanceName, nativeID, cleanup := createTestInstance(t, ctx)
	defer cleanup()

	instanceProv, err := NewBigtableProvisioner(testutil.Config, InstanceResourceType)
	require.NoError(t, err)

	updatedProperties := map[string]interface{}{
		"name":        instanceName,
		"displayName": "Updated Display Name",
	}

	propsJSON, err := json.Marshal(updatedProperties)
	require.NoError(t, err)

	updateReq := &resource.UpdateRequest{
		NativeID:          nativeID,
		ResourceType:      InstanceResourceType,
		DesiredProperties: propsJSON,
		TargetConfig:      testutil.TargetConfig,
	}

	updateResult, err := instanceProv.Update(ctx, updateReq)
	require.NoError(t, err)
	require.NotNil(t, updateResult)
	require.Equal(t, resource.OperationStatusFailure, updateResult.ProgressResult.OperationStatus)
	require.Equal(t, resource.OperationErrorCodeNotUpdatable, updateResult.ProgressResult.ErrorCode)
	t.Logf("Update correctly returned NotUpdatable status")
}

// TestInstance_Delete_Integration tests deleting a Bigtable instance.
func TestInstance_Delete_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	instanceProv, err := NewBigtableProvisioner(testutil.Config, InstanceResourceType)
	require.NoError(t, err)

	// Create instance first (not using helper since we're testing delete)
	instanceName := fmt.Sprintf("formae-test-bt-%s", strings.ToLower(uuid.New().String()[:8]))

	instanceProperties := map[string]interface{}{
		"name":        instanceName,
		"displayName": "Formae Delete Test",
		"type":        "DEVELOPMENT",
		"clusters": map[string]interface{}{
			"cluster1": map[string]interface{}{
				"location":           testutil.Region + "-a",
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
		nativeID = statusResult.ProgressResult.NativeID
	} else {
		nativeID = createResult.ProgressResult.NativeID
	}

	// Delete the instance
	deleteReq := &resource.DeleteRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
	}

	deleteResult, err := instanceProv.Delete(ctx, deleteReq)
	require.NoError(t, err)
	require.NotNil(t, deleteResult)

	if deleteResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress &&
		deleteResult.ProgressResult.RequestID != "" {
		statusResult, err := testutil.PollUntilComplete(t, ctx, instanceProv,
			deleteResult.ProgressResult.RequestID, testutil.TargetConfig, testutil.PollConfig{
				MaxAttempts:   60,
				CheckInterval: 10 * time.Second,
				ResourceType:  InstanceResourceType,
				OperationName: "Delete",
			})
		require.NoError(t, err)
		require.Equal(t, resource.OperationStatusSuccess, statusResult.ProgressResult.OperationStatus)
	} else {
		require.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus)
	}
	t.Logf("Instance deleted: %s", nativeID)

	// Verify it's deleted
	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := instanceProv.Read(ctx, readReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode)
	t.Logf("Verified instance is deleted (not found)")
}

// TestInstance_Delete_NotFound_Integration tests deleting a non-existent instance.
func TestInstance_Delete_NotFound_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	instanceProv, err := NewBigtableProvisioner(testutil.Config, InstanceResourceType)
	require.NoError(t, err)

	nonExistentID := fmt.Sprintf("projects/%s/instances/formae-nonexistent-%s",
		testutil.Project, strings.ToLower(uuid.New().String()[:8]))

	deleteReq := &resource.DeleteRequest{
		NativeID:     nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	deleteResult, err := instanceProv.Delete(ctx, deleteReq)
	require.NoError(t, err)
	require.NotNil(t, deleteResult)
	// Bigtable returns Failure for non-existent resources
	require.Equal(t, resource.OperationStatusFailure, deleteResult.ProgressResult.OperationStatus)
	t.Logf("Delete non-existent instance returned status: %s", deleteResult.ProgressResult.OperationStatus)
}

// TestInstance_List_Integration tests listing Bigtable instances.
func TestInstance_List_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	_, nativeID, cleanup := createTestInstance(t, ctx)
	defer cleanup()

	instanceProv, err := NewBigtableProvisioner(testutil.Config, InstanceResourceType)
	require.NoError(t, err)

	listReq := &resource.ListRequest{
		ResourceType: InstanceResourceType,
		TargetConfig: testutil.TargetConfig,
	}

	listResult, err := instanceProv.List(ctx, listReq)
	require.NoError(t, err)
	require.NotNil(t, listResult)
	require.NotEmpty(t, listResult.NativeIDs)
	t.Logf("Listed %d instances", len(listResult.NativeIDs))

	// Verify our instance is in the list
	found := false
	for _, id := range listResult.NativeIDs {
		if id == nativeID {
			found = true
			break
		}
	}
	assert.True(t, found, "Created instance should be in the list")
}

// TestInstance_CreateInvalid_Integration tests error handling for invalid create requests.
func TestInstance_CreateInvalid_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	instanceProv, err := NewBigtableProvisioner(testutil.Config, InstanceResourceType)
	require.NoError(t, err)

	// Test with missing name
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
}

// TestInstance_Read_NotFound_Integration tests reading a non-existent instance.
func TestInstance_Read_NotFound_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	instanceProv, err := NewBigtableProvisioner(testutil.Config, InstanceResourceType)
	require.NoError(t, err)

	nonExistentID := fmt.Sprintf("projects/%s/instances/formae-nonexistent-%s",
		testutil.Project, strings.ToLower(uuid.New().String()[:8]))

	readReq := &resource.ReadRequest{
		NativeID:     nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := instanceProv.Read(ctx, readReq)
	require.NoError(t, err)
	require.NotNil(t, readResult)
	require.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode)
	t.Logf("Correctly handled read of non-existent instance")
}
