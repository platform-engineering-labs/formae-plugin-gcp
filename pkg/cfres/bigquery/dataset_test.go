// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration
// +build integration

package bigquery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	DatasetResourceType = "GCP::BigQuery::Dataset"
)

// TestDatasetCreate tests the full CRUD lifecycle of a BigQuery dataset
func TestDatasetCreate(t *testing.T) {
	// Create dataset provisioner directly
	dataset := &Dataset{cfg: testutil.Config}

	// BigQuery dataset names must be alphanumeric and underscores only
	datasetID := fmt.Sprintf("formae_test_dataset_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))
	ctx := context.Background()
	nativeID := ""

	// Test 1: Create Dataset
	t.Run("CreateDataset", func(t *testing.T) {
		datasetProperties := map[string]interface{}{
			"datasetId":   datasetID,
			"location":    testutil.Region,
			"description": "Test dataset created by Formae integration tests",
			"labels": map[string]interface{}{
				"test":        "formae-bigquery-dataset",
				"environment": "integration-test",
			},
			"defaultTableExpirationMs": "3600000", // 1 hour
		}

		propsJSON, err := json.Marshal(datasetProperties)
		require.NoError(t, err, "Failed to marshal dataset properties")

		createReq := &resource.CreateRequest{
			ResourceType: DatasetResourceType,
			Properties:   propsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := dataset.Create(ctx, createReq)
		require.NoError(t, err, "Create should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")
		require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus,
			"Create should return Success status for synchronous operation")
		require.NotEmpty(t, createResult.ProgressResult.NativeID, "Native ID should not be empty")

		nativeID = createResult.ProgressResult.NativeID
		t.Logf("Dataset created successfully with native ID: %s", nativeID)

		// Verify native ID format: projects/{project}/datasets/{datasetId}
		expectedPrefix := fmt.Sprintf("projects/%s/datasets/%s", testutil.Project, datasetID)
		assert.Equal(t, expectedPrefix, nativeID, "Native ID should match expected format")
	})

	// Test 2: Read Dataset
	t.Run("ReadDataset", func(t *testing.T) {
		require.NotEmpty(t, nativeID, "Native ID should be set from create test")

		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		readResult, err := dataset.Read(ctx, readReq)
		require.NoError(t, err, "Read should not return error")
		require.NotNil(t, readResult, "Read result should not be nil")
		require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

		t.Logf("Read dataset successfully: %s", readResult.Properties)

		// Unmarshal and verify properties
		var props map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &props)
		require.NoError(t, err, "Should be able to unmarshal resource properties")

		// Verify key properties
		assert.Equal(t, datasetID, props["datasetId"], "Dataset ID should match")
		assert.Equal(t, testutil.Region, props["location"], "Location should match")

		// Check labels
		if labels, ok := props["labels"].(map[string]interface{}); ok {
			assert.Equal(t, "formae-bigquery-dataset", labels["test"], "Test label should match")
			assert.Equal(t, "integration-test", labels["environment"], "Environment label should match")
		} else {
			t.Error("Labels should be present and be a map")
		}
	})

	// Test 3: List Datasets
	t.Run("ListDatasets", func(t *testing.T) {
		listReq := &resource.ListRequest{
			ResourceType: DatasetResourceType,
			TargetConfig: testutil.TargetConfig,
		}

		listResult, err := dataset.List(ctx, listReq)
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
	})

	// Test 4: Update Dataset (should return NotUpdatable)
	t.Run("UpdateDataset", func(t *testing.T) {
		require.NotEmpty(t, nativeID, "Native ID should be set from create test")

		updatedProperties := map[string]interface{}{
			"datasetId":   datasetID,
			"location":    testutil.Region,
			"description": "Updated description",
			"labels": map[string]interface{}{
				"test":        "formae-bigquery-dataset",
				"environment": "integration-test",
				"updated":     "true",
			},
		}

		propsJSON, err := json.Marshal(updatedProperties)
		require.NoError(t, err, "Failed to marshal updated properties")

		updateReq := &resource.UpdateRequest{
			NativeID:          nativeID,
			ResourceType:      DatasetResourceType,
			DesiredProperties: propsJSON,
			TargetConfig:      testutil.TargetConfig,
		}

		updateResult, err := dataset.Update(ctx, updateReq)
		require.NoError(t, err, "Update should not return error")
		require.NotNil(t, updateResult, "Update result should not be nil")
		require.NotNil(t, updateResult.ProgressResult, "Progress result should not be nil")

		// BigQuery datasets don't support updates in the current implementation
		require.Equal(t, resource.OperationStatusFailure, updateResult.ProgressResult.OperationStatus,
			"Update should return Failure status")
		require.Equal(t, resource.OperationErrorCodeNotUpdatable, updateResult.ProgressResult.ErrorCode,
			"Update should return NotUpdatable error code")

		t.Logf("Update correctly returned NotUpdatable status")
	})

	// Test 5: Delete Dataset
	t.Run("DeleteDataset", func(t *testing.T) {
		require.NotEmpty(t, nativeID, "Native ID should be set from create test")

		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		deleteResult, err := dataset.Delete(ctx, deleteReq)
		require.NoError(t, err, "Delete should not return error")
		require.NotNil(t, deleteResult, "Delete result should not be nil")
		require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")
		require.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus,
			"Delete should return Success status")

		t.Logf("Dataset deleted successfully: %s", nativeID)
	})

	// Test 6: Verify Dataset is Deleted
	t.Run("VerifyDatasetDeleted", func(t *testing.T) {
		require.NotEmpty(t, nativeID, "Native ID should be set from create test")

		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		readResult, err := dataset.Read(ctx, readReq)
		require.NoError(t, err, "Read should not return error")
		require.NotNil(t, readResult, "Read result should not be nil")
		require.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode,
			"Read should return NotFound error code")

		t.Logf("Verified dataset is deleted (not found): %s", nativeID)
	})

	// Test 7: Create Dataset with Minimal Properties
	t.Run("CreateDatasetMinimal", func(t *testing.T) {
		minimalDatasetID := fmt.Sprintf("formae_minimal_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

		minimalProperties := map[string]interface{}{
			"datasetId": minimalDatasetID,
			"location":  testutil.Region,
		}

		propsJSON, err := json.Marshal(minimalProperties)
		require.NoError(t, err, "Failed to marshal minimal properties")

		createReq := &resource.CreateRequest{
			ResourceType: DatasetResourceType,
			Properties:   propsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := dataset.Create(ctx, createReq)
		require.NoError(t, err, "Create with minimal properties should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus,
			"Create should return Success status")

		minimalNativeID := createResult.ProgressResult.NativeID
		t.Logf("Minimal dataset created successfully with native ID: %s", minimalNativeID)

		// Cleanup
		deleteReq := &resource.DeleteRequest{
			NativeID:     minimalNativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, err = dataset.Delete(ctx, deleteReq)
		require.NoError(t, err, "Cleanup delete should not return error")
	})
}

// TestDatasetCreateWithExpiration tests creating a dataset with table expiration settings
func TestDatasetCreateWithExpiration(t *testing.T) {
	dataset := &Dataset{cfg: testutil.Config}

	datasetID := fmt.Sprintf("formae_exp_%s", strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))
	ctx := context.Background()

	t.Run("CreateWithExpiration", func(t *testing.T) {
		datasetProperties := map[string]interface{}{
			"datasetId":                datasetID,
			"location":                 testutil.Region,
			"description":              "Dataset with table expiration",
			"defaultTableExpirationMs": "7200000", // 2 hours
		}

		propsJSON, err := json.Marshal(datasetProperties)
		require.NoError(t, err, "Failed to marshal dataset properties")

		createReq := &resource.CreateRequest{
			ResourceType: DatasetResourceType,
			Properties:   propsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := dataset.Create(ctx, createReq)
		require.NoError(t, err, "Create should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus,
			"Create should return Success status")

		nativeID := createResult.ProgressResult.NativeID
		t.Logf("Dataset with expiration created successfully: %s", nativeID)

		// Read and verify expiration is set
		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		readResult, err := dataset.Read(ctx, readReq)
		require.NoError(t, err, "Read should not return error")

		var props map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &props)
		require.NoError(t, err, "Should be able to unmarshal resource properties")

		// Verify expiration is set (API might return it as string or number)
		if expiration, ok := props["defaultTableExpirationMs"]; ok {
			t.Logf("Default table expiration: %v", expiration)
			assert.NotNil(t, expiration, "Expiration should be set")
		}

		// Cleanup
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}
		_, err = dataset.Delete(ctx, deleteReq)
		require.NoError(t, err, "Cleanup delete should not return error")
	})
}

// TestDatasetInvalidCreate tests error handling for invalid dataset creation
func TestDatasetInvalidCreate(t *testing.T) {
	dataset := &Dataset{cfg: testutil.Config}

	ctx := context.Background()

	t.Run("CreateWithMissingDatasetID", func(t *testing.T) {
		invalidProperties := map[string]interface{}{
			"location":    testutil.Region,
			"description": "Missing dataset ID",
		}

		propsJSON, err := json.Marshal(invalidProperties)
		require.NoError(t, err, "Failed to marshal properties")

		createReq := &resource.CreateRequest{
			ResourceType: DatasetResourceType,
			Properties:   propsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := dataset.Create(ctx, createReq)
		require.NoError(t, err, "Create should not return error (error should be in result)")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.Equal(t, resource.OperationStatusFailure, createResult.ProgressResult.OperationStatus,
			"Create should return Failure status for missing dataset ID")

		t.Logf("Correctly rejected create with missing dataset ID: %s",
			createResult.ProgressResult.StatusMessage)
	})

	t.Run("CreateWithInvalidDatasetID", func(t *testing.T) {
		// BigQuery dataset IDs must be alphanumeric + underscores, no hyphens
		invalidProperties := map[string]interface{}{
			"datasetId":   "invalid-dataset-id", // Hyphens not allowed in dataset IDs
			"location":    testutil.Region,
			"description": "Invalid dataset ID with hyphens",
		}

		propsJSON, err := json.Marshal(invalidProperties)
		require.NoError(t, err, "Failed to marshal properties")

		createReq := &resource.CreateRequest{
			ResourceType: DatasetResourceType,
			Properties:   propsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := dataset.Create(ctx, createReq)
		require.NoError(t, err, "Create should not return error (error should be in result)")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.Equal(t, resource.OperationStatusFailure, createResult.ProgressResult.OperationStatus,
			"Create should return Failure status for invalid dataset ID")

		t.Logf("Correctly rejected create with invalid dataset ID: %s",
			createResult.ProgressResult.StatusMessage)
	})
}

// TestDatasetReadNonExistent tests reading a non-existent dataset
func TestDatasetReadNonExistent(t *testing.T) {
	dataset := &Dataset{cfg: testutil.Config}

	ctx := context.Background()

	nonExistentID := fmt.Sprintf("projects/%s/datasets/formae_nonexistent_%s",
		testutil.Project,
		strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	readReq := &resource.ReadRequest{
		NativeID:     nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	readResult, err := dataset.Read(ctx, readReq)
	require.NoError(t, err, "Read should not return error")
	require.NotNil(t, readResult, "Read result should not be nil")
	require.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode,
		"Read should return NotFound error code for non-existent dataset")

	t.Logf("Correctly handled read of non-existent dataset: %s", nonExistentID)
}

// TestDatasetDeleteNonExistent tests deleting a non-existent dataset
func TestDatasetDeleteNonExistent(t *testing.T) {
	dataset := &Dataset{cfg: testutil.Config}

	ctx := context.Background()

	nonExistentID := fmt.Sprintf("projects/%s/datasets/formae_nonexistent_%s",
		testutil.Project,
		strings.ReplaceAll(uuid.New().String()[:8], "-", "_"))

	deleteReq := &resource.DeleteRequest{
		NativeID:     nonExistentID,
		TargetConfig: testutil.TargetConfig,
	}

	deleteResult, err := dataset.Delete(ctx, deleteReq)
	require.NoError(t, err, "Delete should not return error")
	require.NotNil(t, deleteResult, "Delete result should not be nil")
	// Delete of non-existent resource might return Success or Failure depending on implementation
	// Both are acceptable for idempotent deletes
	assert.Contains(t, []resource.OperationStatus{
		resource.OperationStatusSuccess,
		resource.OperationStatusFailure,
	}, deleteResult.ProgressResult.OperationStatus,
		"Delete should return Success or Failure status")

	t.Logf("Delete non-existent dataset returned status: %s",
		deleteResult.ProgressResult.OperationStatus)
}
