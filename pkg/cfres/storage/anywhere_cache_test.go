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

// TestAnywhereCacheCreate tests the creation, reading, and deletion of a GCP Storage AnywhereCache
func TestAnywhereCacheCreate(t *testing.T) {
	t.Skip("Skipping this test for now because AnywhereCache requires premium billing")

	// Create provisioner instance
	anywhereCache, err := NewStorageProvisioner(testutil.Config, "GCP::Storage::AnywhereCache")
	require.NoError(t, err, "Failed to create StorageProvisioner")

	// Create a test bucket first
	bucket, err := NewStorageProvisioner(testutil.Config, "GCP::Storage::Bucket")
	require.NoError(t, err, "Failed to create bucket provisioner")

	bucketName := fmt.Sprintf("formae-test-cache-%s", uuid.New().String()[:8])
	zone := fmt.Sprintf("%s-a", testutil.Region) // e.g., us-central1-a
	t.Logf("Creating test bucket: %s with anywhere cache in zone: %s", bucketName, zone)

	ctx := context.Background()

	// Create bucket
	bucketProperties := map[string]interface{}{
		"name":     bucketName,
		"location": testutil.Region,
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

	// Test AnywhereCache Create operation
	t.Run("CreateAnywhereCache", func(t *testing.T) {
		anywhereCacheProperties := map[string]interface{}{
			"bucket":          bucketName,
			"zone":            zone,
			"admissionPolicy": "admit-on-second-miss",
			"ttl":             "7200s", // 2 hours
		}

		anywhereCachePropsJSON, err := json.Marshal(anywhereCacheProperties)
		require.NoError(t, err, "Failed to marshal anywhere cache properties")

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Storage::AnywhereCache",
			Properties:   anywhereCachePropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		// Create the anywhere cache
		createResult, err := anywhereCache.Create(ctx, createReq)
		require.NoError(t, err, "Create operation should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation, "Operation should be Create")
		assert.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus, "Should be success")
		require.NotEmpty(t, createResult.ProgressResult.NativeID, "NativeID should be set")

		nativeID := createResult.ProgressResult.NativeID
		t.Logf("AnywhereCache created with native ID: %s", nativeID)

		// Test Read operation
		t.Run("Read", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID: nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Storage::AnywhereCache",
			}

			readResult, err := anywhereCache.Read(ctx, readReq)
			require.NoError(t, err, "Read operation should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			require.Empty(t, readResult.ErrorCode, "Read should not have error code")
			require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

			// Verify properties
			readProps, err := utils.ParseProperties(readResult.Properties)
			require.NoError(t, err, "Failed to parse read properties")

			assert.Equal(t, bucketName, utils.GetString(readProps, "bucket"), "Bucket name should match")
			assert.Equal(t, zone, utils.GetString(readProps, "zone"), "Zone should match")
			assert.Equal(t, "admit-on-second-miss", utils.GetString(readProps, "admissionPolicy"), "Admission policy should match")
			assert.Equal(t, "7200s", utils.GetString(readProps, "ttl"), "TTL should match")

			// Verify output-only fields
			assert.NotEmpty(t, utils.GetString(readProps, "anywhereCacheId"), "AnywhereCacheId should be set")
			assert.NotEmpty(t, utils.GetString(readProps, "createTime"), "CreateTime should be set")
			assert.NotEmpty(t, utils.GetString(readProps, "updateTime"), "UpdateTime should be set")
			assert.NotEmpty(t, utils.GetString(readProps, "state"), "State should be set")

			t.Logf("Read anywhere cache properties: %+v", readProps)
		})

		// Test Update operation
		t.Run("Update", func(t *testing.T) {
			updateProperties := map[string]interface{}{
				"ttl": "10800s", // Change TTL to 3 hours
			}

			updatePropsJSON, err := json.Marshal(updateProperties)
			require.NoError(t, err)

			updateReq := &resource.UpdateRequest{
				NativeID:          nativeID,
				ResourceType:      "GCP::Storage::AnywhereCache",
				DesiredProperties: updatePropsJSON,
				TargetConfig:      testutil.TargetConfig,
			}

			updateResult, err := anywhereCache.Update(ctx, updateReq)
			require.NoError(t, err, "Update operation should not return error")
			require.NotNil(t, updateResult, "Update result should not be nil")
			assert.Equal(t, resource.OperationStatusSuccess, updateResult.ProgressResult.OperationStatus, "Should be success")

			// Verify the TTL changed
			readReq := &resource.ReadRequest{
				NativeID: nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Storage::AnywhereCache",
			}

			readResult, err := anywhereCache.Read(ctx, readReq)
			require.NoError(t, err)

			readProps, err := utils.ParseProperties(readResult.Properties)
			require.NoError(t, err)

			assert.Equal(t, "10800s", utils.GetString(readProps, "ttl"), "TTL should be updated")
			t.Logf("AnywhereCache TTL updated to 10800s")
		})

		// Test Delete operation
		t.Run("Delete", func(t *testing.T) {
			deleteReq := &resource.DeleteRequest{
				NativeID: nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Storage::AnywhereCache",
			}

			deleteResult, err := anywhereCache.Delete(ctx, deleteReq)
			require.NoError(t, err, "Delete operation should not return error")
			require.NotNil(t, deleteResult, "Delete result should not be nil")
			require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")

			assert.Equal(t, resource.OperationDelete, deleteResult.ProgressResult.Operation, "Operation should be Delete")
			assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus, "Should be success")

			t.Logf("AnywhereCache deleted successfully")
		})

		// Verify deletion
		t.Run("VerifyDeleted", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID: nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Storage::AnywhereCache",
			}

			readResult, err := anywhereCache.Read(ctx, readReq)
			require.NoError(t, err, "Read should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode, "Should return NotFound")

			t.Logf("Verified anywhere cache was deleted")
		})
	})
}

func TestAnywhereCacheNotFound(t *testing.T) {
	t.Skip("Skipping this test for now because AnywhereCache requires premium billing")
	anywhereCache, err := NewStorageProvisioner(testutil.Config, "GCP::Storage::AnywhereCache")
	require.NoError(t, err)

	readReq := &resource.ReadRequest{
		NativeID:     "formae-test-bucket/anywhereCaches/nonexistent",
		TargetConfig: testutil.TargetConfig,
		ResourceType: "GCP::Storage::AnywhereCache",
	}

	readResult, err := anywhereCache.Read(context.Background(), readReq)
	require.NoError(t, err, "Read should not return error")
	require.NotNil(t, readResult, "Read result should not be nil")
	assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode, "Should return NotFound")

	t.Logf("Verified anywhere cache not found")
}
