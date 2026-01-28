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

// TestBucketCreate tests the creation, reading, and deletion of a GCP Storage Bucket
func TestBucketCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create provisioner instance
	bucket, err := NewStorageProvisioner(testutil.Config, "GCP::Storage::Bucket")
	require.NoError(t, err, "Failed to create StorageProvisioner")

	// Generate unique name (bucket names must be globally unique and lowercase)
	bucketName := fmt.Sprintf("formae-test-bucket-%s", uuid.New().String()[:8])
	t.Logf("Creating test bucket: %s", bucketName)

	ctx := context.Background()

	targetConfig := func() json.RawMessage {
		b, _ := json.Marshal(map[string]interface{}{
			"Project": testutil.Project,
			"Region":  testutil.Region,
		})
		return b
	}()

	// Test Bucket Create operation
	t.Run("CreateBucket", func(t *testing.T) {
		bucketProperties := map[string]interface{}{
			"name":         bucketName,
			"location":     "US",
			"storageClass": "STANDARD",
			"labels": map[string]string{
				"environment": "test",
				"team":        "platform",
			},
			"versioning": map[string]interface{}{
				"enabled": true,
			},
			"lifecycleRule": []map[string]interface{}{
				{
					"action": map[string]interface{}{
						"type": "Delete",
					},
					"condition": map[string]interface{}{
						"age":       30,
						"withState": "ANY",
					},
				},
			},
			"uniformBucketLevelAccess": true,
			"publicAccessPrevention":   "enforced",
		}

		bucketPropsJSON, err := json.Marshal(bucketProperties)
		require.NoError(t, err, "Failed to marshal bucket properties")

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Storage::Bucket",
			Properties:   bucketPropsJSON,
			TargetConfig: targetConfig,
		}

		// Create the bucket (synchronous operation)
		createResult, err := bucket.Create(ctx, createReq)
		require.NoError(t, err, "Create operation should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation, "Operation should be Create")
		assert.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus, "Should be success (synchronous)")
		require.NotEmpty(t, createResult.ProgressResult.NativeID, "NativeID should be set")
		assert.Equal(t, bucketName, createResult.ProgressResult.NativeID, "NativeID should match bucket name")

		nativeID := createResult.ProgressResult.NativeID
		t.Logf("Bucket created with native ID: %s", nativeID)

		// Test Read operation
		t.Run("Read", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: targetConfig,
				ResourceType: "GCP::Storage::Bucket",
			}

			readResult, err := bucket.Read(ctx, readReq)
			require.NoError(t, err, "Read operation should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			require.Empty(t, readResult.ErrorCode, "Read should not have error code")
			require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

			// Verify properties
			readProps, err := utils.ParseProperties(readResult.Properties)
			require.NoError(t, err, "Failed to parse read properties")

			assert.Equal(t, bucketName, utils.GetString(readProps, "name"), "Bucket name should match")
			assert.Equal(t, "US", utils.GetString(readProps, "location"), "Location should match")
			assert.Equal(t, "STANDARD", utils.GetString(readProps, "storageClass"), "Storage class should match")

			// Verify labels
			labels := utils.GetObject(readProps, "labels")
			require.NotNil(t, labels, "Labels should exist")
			assert.Equal(t, "test", utils.GetString(labels, "environment"), "Environment label should match")

			// Verify versioning
			versioning := utils.GetObject(readProps, "versioning")
			require.NotNil(t, versioning, "Versioning should exist")
			assert.True(t, utils.GetBool(versioning, "enabled"), "Versioning should be enabled")

			// Verify lifecycle rules
			lifecycleRules := utils.GetArray(readProps, "lifecycleRule")
			require.NotEmpty(t, lifecycleRules, "Lifecycle rules should not be empty")
			assert.Len(t, lifecycleRules, 1, "Should have 1 lifecycle rule")

			// Verify IAM configuration
			assert.True(t, utils.GetBool(readProps, "uniformBucketLevelAccess"), "Uniform bucket level access should be enabled")
			assert.Equal(t, "enforced", utils.GetString(readProps, "publicAccessPrevention"), "Public access prevention should be enforced")

			// Verify output-only fields
			assert.NotEmpty(t, utils.GetString(readProps, "selfLink"), "SelfLink should be set")
			assert.NotEmpty(t, utils.GetString(readProps, "timeCreated"), "TimeCreated should be set")

			t.Logf("Read bucket properties: %+v", readProps)
		})

		// Test List operation
		t.Run("List", func(t *testing.T) {
			listReq := &resource.ListRequest{
				ResourceType: "GCP::Storage::Bucket",
				TargetConfig: targetConfig,
			}

			listResult, err := bucket.List(ctx, listReq)
			require.NoError(t, err, "List operation should not return error")
			require.NotNil(t, listResult, "List result should not be nil")
			require.NotNil(t, listResult.NativeIDs, "Resources list should not be nil")

			t.Logf("Found %d buckets in project %s", len(listResult.NativeIDs), testutil.Project)

			// Verify our bucket is in the list
			found := false
			for _, id := range listResult.NativeIDs {
				if id == bucketName {
					found = true
					break
				}
			}
			assert.True(t, found, "Created bucket should be in the list")
		})

		// Test Delete operation
		t.Run("Delete", func(t *testing.T) {
			deleteReq := &resource.DeleteRequest{
				NativeID:     nativeID,
				TargetConfig: targetConfig,
				ResourceType: "GCP::Storage::Bucket",
			}

			deleteResult, err := bucket.Delete(ctx, deleteReq)
			require.NoError(t, err, "Delete operation should not return error")
			require.NotNil(t, deleteResult, "Delete result should not be nil")
			require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")

			assert.Equal(t, resource.OperationDelete, deleteResult.ProgressResult.Operation, "Operation should be Delete")
			assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus, "Should be success (synchronous)")

			t.Logf("Bucket deleted successfully")
		})

		// Verify deletion
		t.Run("VerifyDeleted", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: targetConfig,
				ResourceType: "GCP::Storage::Bucket",
			}

			readResult, err := bucket.Read(ctx, readReq)
			require.NoError(t, err, "Read should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode, "Should return NotFound")

			t.Logf("Verified bucket was deleted")
		})
	})
}

func TestBucketNotFound(t *testing.T) {

	bucket, err := NewStorageProvisioner(testutil.Config, "GCP::Storage::Bucket")
	readReq := &resource.ReadRequest{
		NativeID:     "formae-test-bucket-13337",
		TargetConfig: testutil.TargetConfig,
		ResourceType: "GCP::Storage::Bucket",
	}

	readResult, err := bucket.Read(context.Background(), readReq)
	require.NoError(t, err, "Read should not return error")
	require.NotNil(t, readResult, "Read result should not be nil")
	assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode, "Should return NotFound")

	t.Logf("Verified bucket was deleted")
}

// TestBucketWithCORS tests creating a bucket with CORS configuration
func TestBucketWithCORS(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	bucket, err := NewStorageProvisioner(testutil.Config, "GCP::Storage::Bucket")
	require.NoError(t, err, "Failed to create StorageProvisioner")

	bucketName := fmt.Sprintf("formae-test-cors-%s", uuid.New().String()[:8])
	t.Logf("Creating test bucket with CORS: %s", bucketName)

	ctx := context.Background()

	targetConfig := func() json.RawMessage {
		b, _ := json.Marshal(map[string]interface{}{
			"Project": testutil.Project,
			"Region":  testutil.Region,
		})
		return b
	}()

	t.Run("CreateBucketWithCORS", func(t *testing.T) {
		bucketProperties := map[string]interface{}{
			"name":     bucketName,
			"location": "US",
			"cors": []map[string]interface{}{
				{
					"origin":         []string{"https://example.com", "https://app.example.com"},
					"method":         []string{"GET", "POST", "PUT"},
					"responseHeader": []string{"Content-Type", "Authorization"},
					"maxAgeSeconds":  3600,
				},
			},
		}

		bucketPropsJSON, err := json.Marshal(bucketProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Storage::Bucket",
			Properties:   bucketPropsJSON,
			TargetConfig: targetConfig,
		}

		createResult, err := bucket.Create(ctx, createReq)
		require.NoError(t, err)
		require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

		nativeID := createResult.ProgressResult.NativeID

		// Verify CORS configuration
		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: targetConfig,
			ResourceType: "GCP::Storage::Bucket",
		}

		readResult, err := bucket.Read(ctx, readReq)
		require.NoError(t, err)

		readProps := utils.MustParseProperties(readResult.Properties)
		corsRules := utils.GetArray(readProps, "cors")
		require.NotEmpty(t, corsRules, "CORS rules should not be empty")
		assert.Len(t, corsRules, 1, "Should have 1 CORS rule")

		if len(corsRules) > 0 {
			corsRule := corsRules[0].(map[string]interface{})
			origins := utils.GetArray(corsRule, "origin")
			assert.Len(t, origins, 2, "Should have 2 origins")
		}

		t.Logf("CORS bucket created successfully")

		// Cleanup
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: targetConfig,
			ResourceType: "GCP::Storage::Bucket",
		}
		_, err = bucket.Delete(ctx, deleteReq)
		require.NoError(t, err)
	})
}

// TestBucketWithWebsite tests creating a bucket with website configuration
func TestBucketWithWebsite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	bucket, err := NewStorageProvisioner(testutil.Config, "GCP::Storage::Bucket")
	require.NoError(t, err, "Failed to create StorageProvisioner")
	bucketName := fmt.Sprintf("formae-test-website-%s", uuid.New().String()[:8])

	ctx := context.Background()

	targetConfig := func() json.RawMessage {
		b, _ := json.Marshal(map[string]interface{}{
			"Project": testutil.Project,
			"Region":  testutil.Region,
		})
		return b
	}()

	t.Run("CreateBucketWithWebsite", func(t *testing.T) {
		bucketProperties := map[string]interface{}{
			"name":     bucketName,
			"location": "US",
			"website": map[string]interface{}{
				"mainPageSuffix": "index.html",
				"notFoundPage":   "404.html",
			},
		}

		bucketPropsJSON, err := json.Marshal(bucketProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Storage::Bucket",
			Properties:   bucketPropsJSON,
			TargetConfig: targetConfig,
		}

		createResult, err := bucket.Create(ctx, createReq)
		require.NoError(t, err)
		require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

		nativeID := createResult.ProgressResult.NativeID

		// Verify website configuration
		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: targetConfig,
			ResourceType: "GCP::Storage::Bucket",
		}

		readResult, err := bucket.Read(ctx, readReq)
		require.NoError(t, err)

		readProps := utils.MustParseProperties(readResult.Properties)
		website := utils.GetObject(readProps, "website")
		require.NotNil(t, website, "Website configuration should exist")
		assert.Equal(t, "index.html", utils.GetString(website, "mainPageSuffix"), "Main page suffix should match")
		assert.Equal(t, "404.html", utils.GetString(website, "notFoundPage"), "Not found page should match")

		t.Logf("Website bucket created successfully")

		// Cleanup
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: targetConfig,
			ResourceType: "GCP::Storage::Bucket",
		}
		_, err = bucket.Delete(ctx, deleteReq)
		require.NoError(t, err)
	})
}

// TestBucketWithRetentionPolicy tests creating a bucket with retention policy
func TestBucketWithRetentionPolicy(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	bucket, err := NewStorageProvisioner(testutil.Config, "GCP::Storage::Bucket")
	require.NoError(t, err, "Failed to create StorageProvisioner")

	bucketName := fmt.Sprintf("formae-test-retention-%s", uuid.New().String()[:8])

	ctx := context.Background()

	targetConfig := func() json.RawMessage {
		b, _ := json.Marshal(map[string]interface{}{
			"Project": testutil.Project,
			"Region":  testutil.Region,
		})
		return b
	}()

	t.Run("CreateBucketWithRetentionPolicy", func(t *testing.T) {
		bucketProperties := map[string]interface{}{
			"name":     bucketName,
			"location": "US",
			"retentionPolicy": map[string]interface{}{
				"retentionPeriod": 86400, // 1 day in seconds
			},
		}

		bucketPropsJSON, err := json.Marshal(bucketProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Storage::Bucket",
			Properties:   bucketPropsJSON,
			TargetConfig: targetConfig,
		}

		createResult, err := bucket.Create(ctx, createReq)
		require.NoError(t, err)
		require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

		nativeID := createResult.ProgressResult.NativeID

		// Verify retention policy
		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: targetConfig,
			ResourceType: "GCP::Storage::Bucket",
		}

		readResult, err := bucket.Read(ctx, readReq)
		require.NoError(t, err)

		readProps := utils.MustParseProperties(readResult.Properties)
		retentionPolicy := utils.GetObject(readProps, "retentionPolicy")
		require.NotNil(t, retentionPolicy, "Retention policy should exist")
		assert.Equal(t, int64(86400), utils.GetInt64(retentionPolicy, "retentionPeriod"), "Retention period should match")

		t.Logf("Retention policy bucket created successfully")

		// Cleanup
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: targetConfig,
			ResourceType: "GCP::Storage::Bucket",
		}
		_, err = bucket.Delete(ctx, deleteReq)
		require.NoError(t, err)
	})
}

// TestBucketMultiRegion tests creating a bucket in different locations
func TestBucketMultiRegion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	bucket, err := NewStorageProvisioner(testutil.Config, "GCP::Storage::Bucket")
	require.NoError(t, err, "Failed to create StorageProvisioner")

	bucketName := fmt.Sprintf("formae-test-multiregion-%s", uuid.New().String()[:8])

	ctx := context.Background()

	targetConfig := func() json.RawMessage {
		b, _ := json.Marshal(map[string]interface{}{
			"Project": testutil.Project,
			"Region":  testutil.Region,
		})
		return b
	}()

	t.Run("CreateMultiRegionBucket", func(t *testing.T) {
		bucketProperties := map[string]interface{}{
			"name":         bucketName,
			"location":     "EU", // Multi-region location
			"storageClass": "STANDARD",
		}

		bucketPropsJSON, err := json.Marshal(bucketProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Storage::Bucket",
			Properties:   bucketPropsJSON,
			TargetConfig: targetConfig,
		}

		createResult, err := bucket.Create(ctx, createReq)
		require.NoError(t, err)
		require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus)

		nativeID := createResult.ProgressResult.NativeID

		// Verify location
		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: targetConfig,
			ResourceType: "GCP::Storage::Bucket",
		}

		readResult, err := bucket.Read(ctx, readReq)
		require.NoError(t, err)

		readProps := utils.MustParseProperties(readResult.Properties)
		assert.Equal(t, "EU", utils.GetString(readProps, "location"), "Location should be EU")

		t.Logf("Multi-region bucket created successfully in EU")

		// Cleanup
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: targetConfig,
			ResourceType: "GCP::Storage::Bucket",
		}
		_, err = bucket.Delete(ctx, deleteReq)
		require.NoError(t, err)
	})
}
