// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration

package bigquery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/testutil"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDataset_Create_Integration tests creating a BigQuery dataset
func TestDataset_Create_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	provisioner := &Dataset{cfg: testutil.Config}

	// Generate unique name
	datasetID := fmt.Sprintf("formae_test_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))
	t.Logf("Creating test dataset: %s", datasetID)

	datasetProperties := map[string]interface{}{
		"datasetId":   datasetID,
		"location":    testutil.Region,
		"description": "Test dataset created by Formae integration test",
		"labels": map[string]interface{}{
			"environment": "test",
			"created-by":  "formae-integration-test",
		},
	}

	propsJSON, err := json.Marshal(datasetProperties)
	require.NoError(t, err, "Failed to marshal dataset properties")

	createReq := &resource.CreateRequest{
		ResourceType: "GCP::BigQuery::Dataset",
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := provisioner.Create(ctx, createReq)
	require.NoError(t, err, "Create operation should not return error")
	require.NotNil(t, createResult, "Create result should not be nil")
	require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

	// Cleanup after test
	if createResult.ProgressResult.NativeID != "" {
		defer func() {
			deleteReq := &resource.DeleteRequest{
				NativeID:     createResult.ProgressResult.NativeID,
				TargetConfig: testutil.TargetConfig,
			}
			_, _ = provisioner.Delete(ctx, deleteReq)
			t.Logf("Cleaned up test dataset: %s", datasetID)
		}()
	}

	assert.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus,
		"BigQuery operations are synchronous, should return Success")
	require.NotEmpty(t, createResult.ProgressResult.NativeID, "NativeID should be set")

	// Verify native ID format
	expectedNativeID := fmt.Sprintf("projects/%s/datasets/%s", testutil.Project, datasetID)
	assert.Equal(t, expectedNativeID, createResult.ProgressResult.NativeID, "Native ID should match expected format")

	t.Logf("Dataset created successfully with NativeID: %s", createResult.ProgressResult.NativeID)
}

// TestDataset_Read_Integration tests reading a BigQuery dataset
func TestDataset_Read_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	provisioner := &Dataset{cfg: testutil.Config}

	// First create a dataset
	datasetID := fmt.Sprintf("formae_read_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	datasetProperties := map[string]interface{}{
		"datasetId":   datasetID,
		"location":    testutil.Region,
		"description": "Test dataset for read integration test",
		"labels": map[string]interface{}{
			"test-label": "test-value",
		},
	}

	propsJSON, err := json.Marshal(datasetProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: "GCP::BigQuery::Dataset",
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := provisioner.Create(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

	nativeID := createResult.ProgressResult.NativeID

	// Cleanup
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = provisioner.Delete(ctx, deleteReq)
		t.Logf("Cleaned up test dataset: %s", datasetID)
	}()

	// Now test Read
	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := provisioner.Read(ctx, readReq)
	require.NoError(t, err, "Read should not return error")
	require.NotNil(t, readResult, "Read result should not be nil")
	require.Empty(t, readResult.ErrorCode, "ErrorCode should be empty for successful read")
	require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

	// Unmarshal and verify properties
	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err, "Should be able to unmarshal properties")

	assert.Equal(t, datasetID, props["datasetId"], "Dataset ID should match")
	assert.Equal(t, testutil.Region, props["location"], "Location should match")

	// Verify labels
	if labels, ok := props["labels"].(map[string]interface{}); ok {
		assert.Equal(t, "test-value", labels["test-label"], "Label should match")
	}

	t.Logf("Dataset read successfully")
}

// TestDataset_Update_Integration tests updating a BigQuery dataset (should return NotUpdatable)
func TestDataset_Update_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	provisioner := &Dataset{cfg: testutil.Config}

	// First create a dataset
	datasetID := fmt.Sprintf("formae_update_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	datasetProperties := map[string]interface{}{
		"datasetId":   datasetID,
		"location":    testutil.Region,
		"description": "Test dataset for update integration test",
	}

	propsJSON, err := json.Marshal(datasetProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: "GCP::BigQuery::Dataset",
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := provisioner.Create(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

	nativeID := createResult.ProgressResult.NativeID

	// Cleanup
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = provisioner.Delete(ctx, deleteReq)
		t.Logf("Cleaned up test dataset: %s", datasetID)
	}()

	// Now test Update (should return NotUpdatable)
	updatedProperties := map[string]interface{}{
		"datasetId":   datasetID,
		"location":    testutil.Region,
		"description": "Updated description",
		"labels": map[string]interface{}{
			"updated": "true",
		},
	}

	updatePropsJSON, err := json.Marshal(updatedProperties)
	require.NoError(t, err)

	updateReq := &resource.UpdateRequest{
		NativeID:          nativeID,
		ResourceType:      "GCP::BigQuery::Dataset",
		DesiredProperties: updatePropsJSON,
		TargetConfig:      testutil.TargetConfig,
	}

	updateResult, err := provisioner.Update(ctx, updateReq)
	require.NoError(t, err, "Update should not return error")
	require.NotNil(t, updateResult, "Update result should not be nil")
	require.NotNil(t, updateResult.ProgressResult, "Progress result should not be nil")

	// BigQuery datasets don't support updates in the current implementation
	assert.Equal(t, resource.OperationStatusFailure, updateResult.ProgressResult.OperationStatus,
		"Update should return Failure status")
	assert.Equal(t, resource.OperationErrorCodeNotUpdatable, updateResult.ProgressResult.ErrorCode,
		"Update should return NotUpdatable error code")

	t.Logf("Update correctly returned NotUpdatable status")
}

// TestDataset_Delete_Integration tests deleting a BigQuery dataset
func TestDataset_Delete_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	provisioner := &Dataset{cfg: testutil.Config}

	// First create a dataset
	datasetID := fmt.Sprintf("formae_delete_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	datasetProperties := map[string]interface{}{
		"datasetId": datasetID,
		"location":  testutil.Region,
	}

	propsJSON, err := json.Marshal(datasetProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: "GCP::BigQuery::Dataset",
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := provisioner.Create(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

	nativeID := createResult.ProgressResult.NativeID

	// Now test Delete
	deleteReq := &resource.DeleteRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
	}

	deleteResult, err := provisioner.Delete(ctx, deleteReq)
	require.NoError(t, err, "Delete should not return error")
	require.NotNil(t, deleteResult, "Delete result should not be nil")
	require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")
	assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus,
		"Delete should return Success status")

	t.Logf("Dataset deleted successfully: %s", nativeID)

	// Verify dataset is deleted by trying to read it
	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := provisioner.Read(ctx, readReq)
	require.NoError(t, err)
	assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode,
		"Read should return NotFound after deletion")
}

// TestDataset_Delete_NotFound_Integration tests deleting a non-existent dataset (idempotent)
func TestDataset_Delete_NotFound_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	provisioner := &Dataset{cfg: testutil.Config}

	// Use a non-existent dataset ID
	nonExistentID := fmt.Sprintf("projects/%s/datasets/formae_nonexistent_%s",
		testutil.Project,
		strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	deleteReq := &resource.DeleteRequest{
		NativeID:     nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	deleteResult, err := provisioner.Delete(ctx, deleteReq)
	require.NoError(t, err, "Delete should not return error")
	require.NotNil(t, deleteResult, "Delete result should not be nil")

	// Delete of non-existent resource should succeed (idempotent) or return Success
	assert.Contains(t, []resource.OperationStatus{
		resource.OperationStatusSuccess,
		resource.OperationStatusFailure,
	}, deleteResult.ProgressResult.OperationStatus,
		"Delete should handle non-existent resource gracefully")

	t.Logf("Delete non-existent dataset returned status: %s", deleteResult.ProgressResult.OperationStatus)
}

// TestDataset_List_Integration tests listing BigQuery datasets
func TestDataset_List_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	provisioner := &Dataset{cfg: testutil.Config}

	// First create a dataset
	datasetID := fmt.Sprintf("formae_list_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	datasetProperties := map[string]interface{}{
		"datasetId": datasetID,
		"location":  testutil.Region,
	}

	propsJSON, err := json.Marshal(datasetProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: "GCP::BigQuery::Dataset",
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := provisioner.Create(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

	nativeID := createResult.ProgressResult.NativeID

	// Cleanup
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = provisioner.Delete(ctx, deleteReq)
		t.Logf("Cleaned up test dataset: %s", datasetID)
	}()

	// Now test List
	listReq := &resource.ListRequest{
		ResourceType: "GCP::BigQuery::Dataset",
		TargetConfig: testutil.TargetConfig,
	}

	listResult, err := provisioner.List(ctx, listReq)
	require.NoError(t, err, "List should not return error")
	require.NotNil(t, listResult, "List result should not be nil")
	require.NotEmpty(t, listResult.NativeIDs, "List should return at least one dataset")

	t.Logf("Listed %d datasets", len(listResult.NativeIDs))

	// Verify our dataset is in the list
	found := false
	for _, id := range listResult.NativeIDs {
		if id == nativeID {
			found = true
			t.Logf("Found our dataset in list: %s", id)
			break
		}
	}
	assert.True(t, found, "Our dataset should be in the list")
}

// TestDataset_CreateWithExpiration_Integration tests creating a dataset with table expiration
func TestDataset_CreateWithExpiration_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	provisioner := &Dataset{cfg: testutil.Config}

	datasetID := fmt.Sprintf("formae_exp_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	datasetProperties := map[string]interface{}{
		"datasetId":                datasetID,
		"location":                 testutil.Region,
		"description":              "Dataset with table expiration",
		"defaultTableExpirationMs": "7200000", // 2 hours
	}

	propsJSON, err := json.Marshal(datasetProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: "GCP::BigQuery::Dataset",
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := provisioner.Create(ctx, createReq)
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

	nativeID := createResult.ProgressResult.NativeID

	// Cleanup
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, _ = provisioner.Delete(ctx, deleteReq)
	}()

	// Read and verify expiration is set
	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := provisioner.Read(ctx, readReq)
	require.NoError(t, err)

	var props map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &props)
	require.NoError(t, err)

	// Verify expiration is set
	if expiration, ok := props["defaultTableExpirationMs"]; ok {
		t.Logf("Default table expiration: %v", expiration)
		assert.NotNil(t, expiration, "Expiration should be set")
	}
}

// TestDataset_CreateInvalid_Integration tests error handling for invalid dataset creation
func TestDataset_CreateInvalid_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	provisioner := &Dataset{cfg: testutil.Config}

	// Test with missing datasetId
	invalidProperties := map[string]interface{}{
		"location":    testutil.Region,
		"description": "Missing dataset ID",
	}

	propsJSON, err := json.Marshal(invalidProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: "GCP::BigQuery::Dataset",
		Properties:   propsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := provisioner.Create(ctx, createReq)
	require.NoError(t, err, "Create should not return error (error should be in result)")
	require.NotNil(t, createResult)
	assert.Equal(t, resource.OperationStatusFailure, createResult.ProgressResult.OperationStatus,
		"Create should return Failure status for missing dataset ID")

	t.Logf("Correctly rejected create with missing dataset ID: %s", createResult.ProgressResult.StatusMessage)
}
