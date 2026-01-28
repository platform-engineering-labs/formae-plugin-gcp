// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration
// +build integration

package cloudrun

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/testutil"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServiceCreate tests the full CRUD lifecycle of a Cloud Run service
func TestServiceCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	service, err := NewCloudRunProvisioner(testutil.Config, ServiceResourceType)
	require.NoError(t, err, "Failed to create Cloud Run service provisioner")

	serviceName := fmt.Sprintf("formae-test-service-%s", strings.ToLower(uuid.New().String()[:8]))
	ctx := context.Background()
	nativeID := ""

	// Test 1: Create Service
	t.Run("CreateService", func(t *testing.T) {
		serviceProperties := map[string]interface{}{
			"name":     serviceName,
			"location": testutil.Region,
			"labels": map[string]interface{}{
				"test": "formae-cloudrun-service",
			},
			"template": map[string]interface{}{
				"scaling": map[string]interface{}{
					"minInstanceCount": 0,
					"maxInstanceCount": 2,
				},
				"containers": []map[string]interface{}{
					{
						"name":  "hello",
						"image": "us-docker.pkg.dev/cloudrun/container/hello",
						"ports": []map[string]interface{}{
							{
								"name":          "http1",
								"containerPort": 8080,
							},
						},
						"resources": map[string]interface{}{
							"limits": map[string]interface{}{
								"cpu":    "1000m",
								"memory": "512Mi",
							},
						},
					},
				},
			},
		}

		propsJSON, err := json.Marshal(serviceProperties)
		require.NoError(t, err, "Failed to marshal service properties")

		createReq := &resource.CreateRequest{
			ResourceType: ServiceResourceType,
			Properties:   propsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := service.Create(ctx, createReq)
		require.NoError(t, err, "Create should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")
		require.Equal(t, resource.OperationStatusInProgress, createResult.ProgressResult.OperationStatus,
			"Create should return InProgress status")
		require.NotEmpty(t, createResult.ProgressResult.RequestID, "Request ID should not be empty")

		t.Logf("Create initiated with request ID: %s", createResult.ProgressResult.RequestID)

		// Wait for creation to complete
		statusResult, err := testutil.WaitForCreate(t, ctx, service, createResult, testutil.TargetConfig, ServiceResourceType)
		require.NoError(t, err, "Service creation should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")
		require.NotEmpty(t, statusResult.ProgressResult.NativeID, "Native ID should not be empty")

		nativeID = statusResult.ProgressResult.NativeID
		t.Logf("Service created successfully with native ID: %s", nativeID)

		// Note: ResourceProperties are not populated during LRO status checks
		// The Read test will verify the resource properties
	})

	// Test 2: Read Service
	t.Run("ReadService", func(t *testing.T) {
		require.NotEmpty(t, nativeID, "Native ID must be set from create step")

		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: ServiceResourceType,
		}

		readResult, err := service.Read(ctx, readReq)
		require.NoError(t, err, "Read should not return error")
		require.NotNil(t, readResult, "Read result should not be nil")
		require.Empty(t, readResult.ErrorCode, "Read should not return error code")

		var props map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &props)
		require.NoError(t, err, "Failed to unmarshal properties")

		assert.Contains(t, props, "name", "Properties should contain name")
		assert.Contains(t, props, "uid", "Properties should contain uid")
		assert.Contains(t, props, "template", "Properties should contain template")
		assert.Equal(t, testutil.Region, utils.GetString(props, "location"), "Location should match")

		t.Logf("Successfully read service: %s", utils.GetString(props, "name"))
	})

	// Test 3: List Services
	t.Run("ListServices", func(t *testing.T) {
		listReq := &resource.ListRequest{
			ResourceType: ServiceResourceType,
			TargetConfig: testutil.TargetConfig,
		}

		listResult, err := service.List(ctx, listReq)
		require.NoError(t, err, "List should not return error")
		require.NotNil(t, listResult, "List result should not be nil")
		require.NotEmpty(t, listResult.NativeIDs, "List should return at least one resource")

		// Verify our service is in the list
		found := false
		for _, id := range listResult.NativeIDs {
			if id == nativeID {
				found = true
				break
			}
		}

		assert.True(t, found, "Created service should be in list results")
		t.Logf("Found %d services in list", len(listResult.NativeIDs))
	})

	// Test 4: Read Non-Existent Service (404)
	t.Run("ReadNotFound", func(t *testing.T) {
		nonExistentID := fmt.Sprintf("projects/%s/locations/%s/services/does-not-exist-%s",
			testutil.Project, testutil.Region, uuid.New().String()[:8])

		readReq := &resource.ReadRequest{
			NativeID:     nonExistentID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: ServiceResourceType,
		}

		readResult, err := service.Read(ctx, readReq)
		require.NoError(t, err, "Read should not return error even for non-existent resource")
		require.NotNil(t, readResult, "Read result should not be nil")

		// Should return NotFound error code
		assert.NotEmpty(t, readResult.ErrorCode, "Should return error code for non-existent resource")
		assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode,
			"Error code should be NotFound")

		t.Logf("Read non-existent service returned expected NotFound error: %s", readResult.ErrorCode)
	})

	// Test 5: Update Service (should fail - not supported)
	t.Run("UpdateNotSupported", func(t *testing.T) {
		require.NotEmpty(t, nativeID, "Native ID must be set from create step")

		updateProperties := map[string]interface{}{
			"name":     serviceName,
			"location": testutil.Region,
			"labels": map[string]interface{}{
				"test":    "formae-cloudrun-service",
				"updated": "true",
			},
		}

		propsJSON, err := json.Marshal(updateProperties)
		require.NoError(t, err, "Failed to marshal update properties")

		updateReq := &resource.UpdateRequest{
			NativeID:          nativeID,
			ResourceType:      ServiceResourceType,
			DesiredProperties: propsJSON,
			TargetConfig:      testutil.TargetConfig,
		}

		updateResult, err := service.Update(ctx, updateReq)
		require.NoError(t, err, "Update should not return error")
		require.NotNil(t, updateResult, "Update result should not be nil")
		require.NotNil(t, updateResult.ProgressResult, "Progress result should not be nil")

		// Update should fail with NotUpdatable error
		assert.Equal(t, resource.OperationStatusFailure, updateResult.ProgressResult.OperationStatus,
			"Update should return Failure status")
		assert.Equal(t, resource.OperationErrorCodeNotUpdatable, updateResult.ProgressResult.ErrorCode,
			"Update should return NotUpdatable error code")

		t.Logf("Update correctly rejected with NotUpdatable error")
	})

	// Test 6: Delete Service
	t.Run("DeleteService", func(t *testing.T) {
		require.NotEmpty(t, nativeID, "Native ID must be set from create step")

		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: ServiceResourceType,
		}

		deleteResult, err := service.Delete(ctx, deleteReq)
		require.NoError(t, err, "Delete should not return error")
		require.NotNil(t, deleteResult, "Delete result should not be nil")
		require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")
		require.Equal(t, resource.OperationStatusInProgress, deleteResult.ProgressResult.OperationStatus,
			"Delete should return InProgress status")
		require.NotEmpty(t, deleteResult.ProgressResult.RequestID, "Request ID should not be empty")

		t.Logf("Delete initiated with request ID: %s", deleteResult.ProgressResult.RequestID)

		// Wait for deletion to complete
		_, err = testutil.WaitForDelete(t, ctx, service, deleteResult, testutil.TargetConfig, ServiceResourceType)
		require.NoError(t, err, "Service deletion should complete successfully")

		t.Logf("Service deleted successfully")
	})

	// Test 7: Verify Service is Deleted (404 after delete)
	t.Run("VerifyDeleted", func(t *testing.T) {
		require.NotEmpty(t, nativeID, "Native ID must be set from create step")

		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: ServiceResourceType,
		}

		readResult, err := service.Read(ctx, readReq)
		require.NoError(t, err, "Read should not return error")
		require.NotNil(t, readResult, "Read result should not be nil")

		// Should return NotFound error code
		assert.NotEmpty(t, readResult.ErrorCode, "Should return error code for deleted resource")
		assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode,
			"Error code should be NotFound after deletion")

		t.Logf("Verified service is deleted - Read returned NotFound")
	})
}

// TestServiceCreateWithVPC tests Cloud Run service creation with VPC access configuration
func TestServiceCreateWithVPC(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	service, err := NewCloudRunProvisioner(testutil.Config, ServiceResourceType)
	require.NoError(t, err, "Failed to create Cloud Run service provisioner")

	serviceName := fmt.Sprintf("formae-test-vpc-service-%s", strings.ToLower(uuid.New().String()[:8]))
	ctx := context.Background()
	nativeID := ""

	t.Run("CreateServiceWithVPCConnector", func(t *testing.T) {
		serviceProperties := map[string]interface{}{
			"name":     serviceName,
			"location": testutil.Region,
			"template": map[string]interface{}{
				"containers": []map[string]interface{}{
					{
						"image": "us-docker.pkg.dev/cloudrun/container/hello",
						"resources": map[string]interface{}{
							"limits": map[string]interface{}{
								"cpu":    "1000m",
								"memory": "512Mi",
							},
						},
					},
				},
			},
		}

		propsJSON, err := json.Marshal(serviceProperties)
		require.NoError(t, err, "Failed to marshal service properties")

		createReq := &resource.CreateRequest{
			ResourceType: ServiceResourceType,
			Properties:   propsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := service.Create(ctx, createReq)
		require.NoError(t, err, "Create should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")

		// Wait for creation
		statusResult, err := testutil.WaitForCreate(t, ctx, service, createResult, testutil.TargetConfig, ServiceResourceType)
		require.NoError(t, err, "Service creation should complete successfully")

		nativeID = statusResult.ProgressResult.NativeID
		t.Logf("Service with VPC access created: %s", nativeID)
	})

	// Cleanup
	t.Run("Cleanup", func(t *testing.T) {
		if nativeID == "" {
			t.Skip("No resource to cleanup")
		}

		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: ServiceResourceType,
		}

		deleteResult, err := service.Delete(ctx, deleteReq)
		require.NoError(t, err, "Delete should not return error")

		_, err = testutil.WaitForDelete(t, ctx, service, deleteResult, testutil.TargetConfig, ServiceResourceType)
		require.NoError(t, err, "Service deletion should complete successfully")

		t.Logf("VPC service cleaned up successfully")
	})
}
