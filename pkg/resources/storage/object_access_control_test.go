// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration
// +build integration

package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/testutil"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	htransport "google.golang.org/api/transport/http"
)

// TestObjectAccessControlCreate tests the creation, reading, and deletion of a GCP Storage ObjectAccessControl
// NOTE: This test requires creating an object first, which is complex with the Storage API.
// Object ACLs are also discouraged in favor of uniform bucket-level access.
// Test is commented out but the implementation is complete.

func TestObjectAccessControlCreate(t *testing.T) {

	t.Skip("Skipping integration: Cannot update access control for an object when uniform bucket-level access is enabled ")

	// Create provisioner instance
	objectACL, err := NewStorageProvisioner(testutil.Config, "GCP::Storage::ObjectAccessControl")
	require.NoError(t, err, "Failed to create StorageProvisioner")

	// Create a test bucket first (without uniform bucket-level access)
	bucket, err := NewStorageProvisioner(testutil.Config, "GCP::Storage::Bucket")
	require.NoError(t, err, "Failed to create bucket provisioner")

	bucketName := fmt.Sprintf("formae-test-objacl-%s", uuid.New().String()[:8])
	objectName := "test-object.txt"
	entity := "allUsers"
	t.Logf("Creating test bucket: %s with object: %s and ACL for entity: %s", bucketName, objectName, entity)

	ctx := context.Background()

	// Create bucket (uniform bucket-level access must be disabled for ACLs)
	bucketProperties := map[string]interface{}{
		"name":     bucketName,
		"location": "US",
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

	// Create transport client for authenticated API calls
	transportClient, err := transport.NewClient(ctx, testutil.Config)
	require.NoError(t, err, "Failed to create transport client")

	// Create a test object in the bucket using direct HTTP upload
	// Note: We can't use SendRequest because it always sends JSON, but media uploads require binary/text
	uploadURL := fmt.Sprintf("https://storage.googleapis.com/upload/storage/v1/b/%s/o?uploadType=media&name=%s", bucketName, objectName)

	// Create upload request with empty content
	uploadReq, err := http.NewRequestWithContext(ctx, "POST", uploadURL, bytes.NewReader([]byte("")))
	require.NoError(t, err, "Failed to create upload request")

	uploadReq.Header.Set("Content-Type", "text/plain")
	uploadReq.Header.Set("Content-Length", "0")

	// Execute upload using the authenticated HTTP client from transport
	// The transport.Client has an unexported httpClient field we need to access via reflection
	// or we can use the public SendRequest method for cleanup but not for upload
	// Instead, we'll create a new authenticated client directly
	opts, err := testutil.Config.ToClientOptions(ctx)
	require.NoError(t, err, "Failed to create client options")

	httpClient, _, err := htransport.NewClient(ctx, opts...)
	require.NoError(t, err, "Failed to create authenticated HTTP client")

	uploadResp, err := httpClient.Do(uploadReq)
	require.NoError(t, err, "Failed to upload object")
	defer uploadResp.Body.Close()

	// Check response
	if uploadResp.StatusCode != 200 {
		body, _ := io.ReadAll(uploadResp.Body)
		t.Fatalf("Object upload failed with status %d: %s", uploadResp.StatusCode, string(body))
	}

	defer func() {
		// Cleanup object
		deleteObjURL := fmt.Sprintf("https://storage.googleapis.com/storage/v1/b/%s/o/%s", bucketName, objectName)
		transportClient.SendRequest(ctx, transport.RequestOptions{
			Method: "DELETE",
			URL:    deleteObjURL,
		})
		// Cleanup bucket
		deleteReq := &resource.DeleteRequest{
			NativeID:     bucketNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Storage::Bucket",
		}
		bucket.Delete(ctx, deleteReq)
	}()

	// Test ObjectAccessControl Create operation
	t.Run("CreateObjectAccessControl", func(t *testing.T) {
		aclProperties := map[string]interface{}{
			"bucket": bucketName,
			"object": objectName,
			"entity": entity,
			"role":   "READER",
		}

		aclPropsJSON, err := json.Marshal(aclProperties)
		require.NoError(t, err, "Failed to marshal object ACL properties")

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Storage::ObjectAccessControl",
			Properties:   aclPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		// Create the object ACL
		createResult, err := objectACL.Create(ctx, createReq)
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
		t.Logf("ObjectAccessControl created with native ID: %s", nativeID)

		// Test Read operation
		t.Run("Read", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Storage::ObjectAccessControl",
			}

			readResult, err := objectACL.Read(ctx, readReq)
			require.NoError(t, err, "Read operation should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			require.Empty(t, readResult.ErrorCode, "Read should not have error code")
			require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

			// Verify properties
			readProps, err := utils.ParseProperties(readResult.Properties)
			require.NoError(t, err, "Failed to parse read properties")

			assert.Equal(t, bucketName, utils.GetString(readProps, "bucket"), "Bucket name should match")
			assert.Equal(t, objectName, utils.GetString(readProps, "object"), "Object name should match")
			assert.Equal(t, entity, utils.GetString(readProps, "entity"), "Entity should match")
			assert.Equal(t, "READER", utils.GetString(readProps, "role"), "Role should match")

			t.Logf("Read object ACL properties: %+v", readProps)
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
				ResourceType:      "GCP::Storage::ObjectAccessControl",
				DesiredProperties: updatePropsJSON,
				TargetConfig:      testutil.TargetConfig,
			}

			updateResult, err := objectACL.Update(ctx, updateReq)
			require.NoError(t, err, "Update operation should not return error")
			require.NotNil(t, updateResult, "Update result should not be nil")
			assert.Equal(t, resource.OperationStatusSuccess, updateResult.ProgressResult.OperationStatus, "Should be success")

			// Verify the role changed
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Storage::ObjectAccessControl",
			}

			readResult, err := objectACL.Read(ctx, readReq)
			require.NoError(t, err)

			readProps, err := utils.ParseProperties(readResult.Properties)
			require.NoError(t, err)

			assert.Equal(t, "OWNER", utils.GetString(readProps, "role"), "Role should be OWNER")
			t.Logf("ObjectAccessControl role updated to OWNER")
		})

		// Test Delete operation
		t.Run("Delete", func(t *testing.T) {
			deleteReq := &resource.DeleteRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Storage::ObjectAccessControl",
			}

			deleteResult, err := objectACL.Delete(ctx, deleteReq)
			require.NoError(t, err, "Delete operation should not return error")
			require.NotNil(t, deleteResult, "Delete result should not be nil")
			require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")

			assert.Equal(t, resource.OperationDelete, deleteResult.ProgressResult.Operation, "Operation should be Delete")
			assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus, "Should be success")

			t.Logf("ObjectAccessControl deleted successfully")
		})

		// Verify deletion
		t.Run("VerifyDeleted", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Storage::ObjectAccessControl",
			}

			readResult, err := objectACL.Read(ctx, readReq)
			require.NoError(t, err, "Read should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode, "Should return NotFound")

			t.Logf("Verified object ACL was deleted")
		})
	})
}

func TestObjectAccessControlNotFound(t *testing.T) {
	// ObjectAccessControl resource type is not yet registered in the storage registry
	// (requires special handling for object-scoped resources with two parent properties)
	t.Skip("Skipping: GCP::Storage::ObjectAccessControl resource type not yet implemented")
}
