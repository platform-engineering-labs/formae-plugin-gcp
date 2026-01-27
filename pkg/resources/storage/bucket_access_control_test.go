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
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/testutil"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBucketAccessControlCreate tests the creation, reading, and deletion of a GCP Storage BucketAccessControl
// NOTE: This test requires disabling uniform bucket-level access, which may be prevented by
// organization policy constraints/storage.uniformBucketLevelAccess.
// ACL-based access control is deprecated in favor of uniform bucket-level access.
func TestBucketAccessControlCreate(t *testing.T) {

	// Create provisioner instance
	bucketACL, err := NewStorageProvisioner(testutil.Config, "GCP::Storage::BucketAccessControl")
	require.NoError(t, err, "Failed to create StorageProvisioner")

	// Create a test bucket first (without uniform bucket-level access)
	bucket, err := NewStorageProvisioner(testutil.Config, "GCP::Storage::Bucket")
	require.NoError(t, err, "Failed to create bucket provisioner")

	bucketName := fmt.Sprintf("formae-test-acl-%s", uuid.New().String()[:8])
	t.Logf("Creating test bucket: %s", bucketName)

	ctx := context.Background()

	// Create bucket (uniform bucket-level access must be disabled for ACLs)
	// By default, GCS grants the authenticated service account OWNER permission
	bucketProperties := map[string]interface{}{
		"name":     bucketName,
		"location": "US",
		"iamConfiguration": map[string]interface{}{
			"uniformBucketLevelAccess": map[string]interface{}{
				"enabled": false,
			},
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

	// Test BucketAccessControl Create operation
	// The authenticated service account has OWNER permission by default, allowing ACL management
	t.Run("CreateBucketAccessControl", func(t *testing.T) {
		// Use project-viewers convenience group to avoid org policy restrictions
		// Note: Convenience groups use project NUMBER, not project ID
		testEntity := fmt.Sprintf("project-viewers-%s", testutil.ProjectNumber)

		aclProperties := map[string]interface{}{
			"bucket": bucketName,
			"entity": testEntity,
			"role":   "READER",
		}

		aclPropsJSON, err := json.Marshal(aclProperties)
		require.NoError(t, err, "Failed to marshal ACL properties")

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Storage::BucketAccessControl",
			Properties:   aclPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		// Create the bucket ACL
		createResult, err := bucketACL.Create(ctx, createReq)
		require.NoError(t, err, "Create operation should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation, "Operation should be Create")
		assert.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus, "Should be success")
		require.NotEmpty(t, createResult.ProgressResult.NativeID, "NativeID should be set")

		nativeID := createResult.ProgressResult.NativeID
		t.Logf("BucketAccessControl created with native ID: %s", nativeID)

		// Test Read operation
		t.Run("Read", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Storage::BucketAccessControl",
			}

			readResult, err := bucketACL.Read(ctx, readReq)
			require.NoError(t, err, "Read operation should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			require.Empty(t, readResult.ErrorCode, "Read should not have error code")
			require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

			// Verify properties
			readProps, err := utils.ParseProperties(readResult.Properties)
			require.NoError(t, err, "Failed to parse read properties")

			assert.Equal(t, bucketName, utils.GetString(readProps, "bucket"), "Bucket name should match")
			assert.Equal(t, testEntity, utils.GetString(readProps, "entity"), "Entity should match")
			assert.Equal(t, "READER", utils.GetString(readProps, "role"), "Role should match")

			t.Logf("Read bucket ACL properties: %+v", readProps)
		})

		// Test Update operation
		t.Run("Update", func(t *testing.T) {
			updateProperties := map[string]interface{}{
				"role": "WRITER",
			}

			updatePropsJSON, err := json.Marshal(updateProperties)
			require.NoError(t, err)

			updateReq := &resource.UpdateRequest{
				NativeID:          nativeID,
				ResourceType:      "GCP::Storage::BucketAccessControl",
				DesiredProperties: updatePropsJSON,
				TargetConfig:      testutil.TargetConfig,
			}

			updateResult, err := bucketACL.Update(ctx, updateReq)
			require.NoError(t, err, "Update operation should not return error")
			require.NotNil(t, updateResult, "Update result should not be nil")
			assert.Equal(t, resource.OperationStatusSuccess, updateResult.ProgressResult.OperationStatus, "Should be success")

			// Verify the role changed
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Storage::BucketAccessControl",
			}

			readResult, err := bucketACL.Read(ctx, readReq)
			require.NoError(t, err)

			readProps, err := utils.ParseProperties(readResult.Properties)
			require.NoError(t, err)

			assert.Equal(t, "WRITER", utils.GetString(readProps, "role"), "Role should be WRITER")
			t.Logf("BucketAccessControl role updated to WRITER")
		})

		// Test Delete operation
		t.Run("Delete", func(t *testing.T) {
			deleteReq := &resource.DeleteRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Storage::BucketAccessControl",
			}

			deleteResult, err := bucketACL.Delete(ctx, deleteReq)
			require.NoError(t, err, "Delete operation should not return error")
			require.NotNil(t, deleteResult, "Delete result should not be nil")
			require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")

			assert.Equal(t, resource.OperationDelete, deleteResult.ProgressResult.Operation, "Operation should be Delete")
			assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus, "Should be success")

			t.Logf("BucketAccessControl deleted successfully")
		})

		// Verify deletion
		t.Run("VerifyDeleted", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Storage::BucketAccessControl",
			}

			readResult, err := bucketACL.Read(ctx, readReq)
			require.NoError(t, err, "Read should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode, "Should return NotFound")

			t.Logf("Verified bucket ACL was deleted")
		})
	})
}

func TestBucketAccessControlNotFound(t *testing.T) {
	bucketACL, err := NewStorageProvisioner(testutil.Config, "GCP::Storage::BucketAccessControl")
	require.NoError(t, err)

	// Use proper NativeID format: b/{bucket}/acl/{entity}
	readReq := &resource.ReadRequest{
		NativeID:     "b/nonexistent-bucket/acl/allUsers",
		TargetConfig: testutil.TargetConfig,
		ResourceType: "GCP::Storage::BucketAccessControl",
	}

	readResult, err := bucketACL.Read(context.Background(), readReq)
	require.NoError(t, err, "Read should not return error")
	require.NotNil(t, readResult, "Read result should not be nil")
	// GCP returns AccessDenied for non-existent buckets (to prevent bucket enumeration)
	// This is expected behavior - either NotFound or AccessDenied is acceptable
	assert.Contains(t, []resource.OperationErrorCode{
		resource.OperationErrorCodeNotFound,
		resource.OperationErrorCodeAccessDenied,
	}, readResult.ErrorCode, "Should return NotFound or AccessDenied for non-existent bucket")

	t.Logf("Verified bucket ACL not found (error: %s)", readResult.ErrorCode)
}
