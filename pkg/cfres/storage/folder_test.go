// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration
// +build integration

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/testutil"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFolderCreate tests the creation, reading, and deletion of a GCP Storage Folder
func TestFolderCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create provisioner instance
	folder, err := NewStorageProvisioner(testutil.Config, "GCP::Storage::Folder")
	require.NoError(t, err, "Failed to create StorageProvisioner")

	// Create a test bucket first with hierarchical namespace enabled
	bucket, err := NewStorageProvisioner(testutil.Config, "GCP::Storage::Bucket")
	require.NoError(t, err, "Failed to create bucket provisioner")

	bucketName := fmt.Sprintf("formae-test-folder-%s", uuid.New().String()[:8])
	folderName := fmt.Sprintf("test-folder-%s/", uuid.New().String()[:8])
	t.Logf("Creating test bucket: %s with folder: %s", bucketName, folderName)

	ctx := context.Background()

	// Create bucket with hierarchical namespace enabled (required for Folder resources)
	bucketProperties := map[string]interface{}{
		"name":     bucketName,
		"location": "US",
		"iamConfiguration": map[string]interface{}{
			"uniformBucketLevelAccess": map[string]interface{}{
				"enabled": true,
			},
		},
		"hierarchicalNamespace": map[string]interface{}{
			"enabled": true,
		},
	}
	bucketPropsJSON, err := json.Marshal(bucketProperties)
	require.NoError(t, err)

	createBucketReq := &resource.CreateRequest{
		ResourceType: "GCP::Storage::Bucket",
		Properties:   bucketPropsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	bucketResult, err := bucket.Create(ctx, createBucketReq)
	require.NoError(t, err, "Failed to create test bucket")
	require.Equal(t, resource.OperationStatusSuccess, bucketResult.ProgressResult.OperationStatus)

	bucketNativeID := bucketResult.ProgressResult.NativeID
	defer func() {
		// Cleanup bucket
		deleteReq := &resource.DeleteRequest{
			NativeID:     bucketNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Storage::Bucket",
		}
		bucket.Delete(ctx, deleteReq)
	}()

	// Test Folder Create operation
	t.Run("CreateFolder", func(t *testing.T) {
		folderProperties := map[string]interface{}{
			"bucket": bucketName,
			"name":   folderName,
		}

		folderPropsJSON, err := json.Marshal(folderProperties)
		require.NoError(t, err, "Failed to marshal folder properties")

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Storage::Folder",
			Properties:   folderPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		// Create the folder
		createResult, err := folder.Create(ctx, createReq)
		require.NoError(t, err, "Create operation should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

		if createResult.ProgressResult.OperationStatus != resource.OperationStatusSuccess {
			t.Logf("Create failed with error: %s (code: %s)", createResult.ProgressResult.StatusMessage, createResult.ProgressResult.ErrorCode)
		}

		assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation, "Operation should be Create")
		assert.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus, "Should be success")
		require.NotEmpty(t, createResult.ProgressResult.NativeID, "NativeID should be set")

		nativeID := createResult.ProgressResult.NativeID
		t.Logf("Folder created with native ID: %s", nativeID)

		// Test Read operation
		t.Run("Read", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Storage::Folder",
			}

			readResult, err := folder.Read(ctx, readReq)
			require.NoError(t, err, "Read operation should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			require.Empty(t, readResult.ErrorCode, "Read should not have error code")
			require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

			// Verify properties
			readProps, err := utils.ParseProperties(readResult.Properties)
			require.NoError(t, err, "Failed to parse read properties")

			assert.Equal(t, bucketName, utils.GetString(readProps, "bucket"), "Bucket name should match")
			assert.Equal(t, folderName, utils.GetString(readProps, "name"), "Folder name should match")

			// Verify output-only fields
			assert.NotEmpty(t, utils.GetString(readProps, "createTime"), "CreateTime should be set")
			assert.NotEmpty(t, utils.GetString(readProps, "updateTime"), "UpdateTime should be set")
			assert.NotEmpty(t, utils.GetString(readProps, "metageneration"), "Metageneration should be set")

			t.Logf("Read folder properties: %+v", readProps)
		})

		// Test Delete operation
		t.Run("Delete", func(t *testing.T) {
			deleteReq := &resource.DeleteRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Storage::Folder",
			}

			deleteResult, err := folder.Delete(ctx, deleteReq)
			require.NoError(t, err, "Delete operation should not return error")
			require.NotNil(t, deleteResult, "Delete result should not be nil")
			require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")

			assert.Equal(t, resource.OperationDelete, deleteResult.ProgressResult.Operation, "Operation should be Delete")
			assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus, "Should be success")

			t.Logf("Folder deleted successfully")
		})

		// Verify deletion
		t.Run("VerifyDeleted", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Storage::Folder",
			}

			readResult, err := folder.Read(ctx, readReq)
			require.NoError(t, err, "Read should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode, "Should return NotFound")

			t.Logf("Verified folder was deleted")
		})
	})
}

func TestFolderNotFound(t *testing.T) {
	folder, err := NewStorageProvisioner(testutil.Config, "GCP::Storage::Folder")
	require.NoError(t, err)

	// Use proper NativeID format: b/{bucket}/folders/{folderName}
	readReq := &resource.ReadRequest{
		NativeID:     "b/nonexistent-bucket/folders/nonexistent-folder/",
		TargetConfig: testutil.TargetConfig,
		ResourceType: "GCP::Storage::Folder",
	}

	readResult, err := folder.Read(context.Background(), readReq)
	require.NoError(t, err, "Read should not return error")
	require.NotNil(t, readResult, "Read result should not be nil")
	// GCP returns AccessDenied for non-existent buckets (to prevent bucket enumeration)
	assert.Contains(t, []resource.OperationErrorCode{
		resource.OperationErrorCodeNotFound,
		resource.OperationErrorCodeAccessDenied,
	}, readResult.ErrorCode, "Should return NotFound or AccessDenied for non-existent bucket")

	t.Logf("Verified folder not found (error: %s)", readResult.ErrorCode)
}
