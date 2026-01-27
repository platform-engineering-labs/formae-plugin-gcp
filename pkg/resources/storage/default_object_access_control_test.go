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

// TestDefaultObjectAccessControlCreate tests the creation, reading, and deletion of a GCP Storage DefaultObjectAccessControl
// NOTE: This test requires disabling uniform bucket-level access, which may be prevented by
// organization policy constraints/storage.uniformBucketLevelAccess.
// ACL-based access control is deprecated in favor of uniform bucket-level access.
func TestDefaultObjectAccessControlCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Skip("Skipping test: requires disabling uniform bucket-level access, which may violate organization policy constraints/storage.uniformBucketLevelAccess")

	// Create provisioner instance
	defaultObjACL, err := NewStorageProvisioner(testutil.Config, "GCP::Storage::DefaultObjectAccessControl")
	require.NoError(t, err, "Failed to create StorageProvisioner")

	// Create a test bucket first (without uniform bucket-level access)
	bucket, err := NewStorageProvisioner(testutil.Config, "GCP::Storage::Bucket")
	require.NoError(t, err, "Failed to create bucket provisioner")

	bucketName := fmt.Sprintf("formae-test-defacl-%s", uuid.New().String()[:8])
	entity := "allAuthenticatedUsers" // Authenticated users for testing
	t.Logf("Creating test bucket: %s with default object ACL for entity: %s", bucketName, entity)

	ctx := context.Background()

	// Create bucket (uniform bucket-level access must be disabled for ACLs)
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

	// Test DefaultObjectAccessControl Create operation
	t.Run("CreateDefaultObjectAccessControl", func(t *testing.T) {
		aclProperties := map[string]interface{}{
			"bucket": bucketName,
			"entity": entity,
			"role":   "READER",
		}

		aclPropsJSON, err := json.Marshal(aclProperties)
		require.NoError(t, err, "Failed to marshal default object ACL properties")

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Storage::DefaultObjectAccessControl",
			Properties:   aclPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		// Create the default object ACL
		createResult, err := defaultObjACL.Create(ctx, createReq)
		require.NoError(t, err, "Create operation should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation, "Operation should be Create")
		assert.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus, "Should be success")
		require.NotEmpty(t, createResult.ProgressResult.NativeID, "NativeID should be set")

		nativeID := createResult.ProgressResult.NativeID
		t.Logf("DefaultObjectAccessControl created with native ID: %s", nativeID)

		// Test Read operation
		t.Run("Read", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Storage::DefaultObjectAccessControl",
			}

			readResult, err := defaultObjACL.Read(ctx, readReq)
			require.NoError(t, err, "Read operation should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			require.Empty(t, readResult.ErrorCode, "Read should not have error code")
			require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

			// Verify properties
			readProps, err := utils.ParseProperties(readResult.Properties)
			require.NoError(t, err, "Failed to parse read properties")

			assert.Equal(t, bucketName, utils.GetString(readProps, "bucket"), "Bucket name should match")
			assert.Equal(t, entity, utils.GetString(readProps, "entity"), "Entity should match")
			assert.Equal(t, "READER", utils.GetString(readProps, "role"), "Role should match")

			t.Logf("Read default object ACL properties: %+v", readProps)
		})

		// Test Update operation
		t.Run("Update", func(t *testing.T) {
			updateProperties := map[string]interface{}{
				"role": "OWNER",
			}

			updatePropsJSON, err := json.Marshal(updateProperties)
			require.NoError(t, err)

			updateReq := &resource.UpdateRequest{
				NativeID:          nativeID,
				ResourceType:      "GCP::Storage::DefaultObjectAccessControl",
				DesiredProperties: updatePropsJSON,
				TargetConfig:      testutil.TargetConfig,
			}

			updateResult, err := defaultObjACL.Update(ctx, updateReq)
			require.NoError(t, err, "Update operation should not return error")
			require.NotNil(t, updateResult, "Update result should not be nil")
			assert.Equal(t, resource.OperationStatusSuccess, updateResult.ProgressResult.OperationStatus, "Should be success")

			// Verify the role changed
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Storage::DefaultObjectAccessControl",
			}

			readResult, err := defaultObjACL.Read(ctx, readReq)
			require.NoError(t, err)

			readProps, err := utils.ParseProperties(readResult.Properties)
			require.NoError(t, err)

			assert.Equal(t, "OWNER", utils.GetString(readProps, "role"), "Role should be OWNER")
			t.Logf("DefaultObjectAccessControl role updated to OWNER")
		})

		// Test Delete operation
		t.Run("Delete", func(t *testing.T) {
			deleteReq := &resource.DeleteRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Storage::DefaultObjectAccessControl",
			}

			deleteResult, err := defaultObjACL.Delete(ctx, deleteReq)
			require.NoError(t, err, "Delete operation should not return error")
			require.NotNil(t, deleteResult, "Delete result should not be nil")
			require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")

			assert.Equal(t, resource.OperationDelete, deleteResult.ProgressResult.Operation, "Operation should be Delete")
			assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus, "Should be success")

			t.Logf("DefaultObjectAccessControl deleted successfully")
		})

		// Verify deletion
		t.Run("VerifyDeleted", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Storage::DefaultObjectAccessControl",
			}

			readResult, err := defaultObjACL.Read(ctx, readReq)
			require.NoError(t, err, "Read should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode, "Should return NotFound")

			t.Logf("Verified default object ACL was deleted")
		})
	})
}

func TestDefaultObjectAccessControlNotFound(t *testing.T) {
	defaultObjACL, err := NewStorageProvisioner(testutil.Config, "GCP::Storage::DefaultObjectAccessControl")
	require.NoError(t, err)

	// Use proper NativeID format: b/{bucket}/defaultObjectAcl/{entity}
	readReq := &resource.ReadRequest{
		NativeID:     "b/nonexistent-bucket/defaultObjectAcl/allUsers",
		TargetConfig: testutil.TargetConfig,
		ResourceType: "GCP::Storage::DefaultObjectAccessControl",
	}

	readResult, err := defaultObjACL.Read(context.Background(), readReq)
	require.NoError(t, err, "Read should not return error")
	require.NotNil(t, readResult, "Read result should not be nil")
	// GCP returns AccessDenied for non-existent buckets (to prevent bucket enumeration)
	// This is expected behavior - either NotFound or AccessDenied is acceptable
	assert.Contains(t, []resource.OperationErrorCode{
		resource.OperationErrorCodeNotFound,
		resource.OperationErrorCodeAccessDenied,
	}, readResult.ErrorCode, "Should return NotFound or AccessDenied for non-existent bucket")

	t.Logf("Verified default object ACL not found (error: %s)", readResult.ErrorCode)
}
