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
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/testutil"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJobCreate tests the full CRUD lifecycle of a Cloud Run job
func TestJobCreate(t *testing.T) {
	job, err := NewCloudRunProvisioner(testutil.Config, JobResourceType)
	require.NoError(t, err, "Failed to create Cloud Run job provisioner")

	// Cloud Run job names must be lowercase, start with letter, no uppercase
	jobName := fmt.Sprintf("formae-test-job-%s", strings.ToLower(uuid.New().String()[:8]))
	ctx := context.Background()
	nativeID := ""

	// Test 1: Create Job
	t.Run("CreateJob", func(t *testing.T) {
		jobProperties := map[string]interface{}{
			"name":     jobName,
			"location": testutil.Region,
			"labels": map[string]interface{}{
				"test": "formae-cloudrun-job",
			},
			"template": map[string]interface{}{
				"taskCount":   1,
				"parallelism": 1,
				"template": map[string]interface{}{
					"maxRetries": 3,
					"timeout":    "600s",
					"containers": []map[string]interface{}{
						{
							"name":  "test-container",
							"image": "us-docker.pkg.dev/cloudrun/container/job",
							"env": []map[string]interface{}{
								{
									"name":  "TEST_ENV",
									"value": "test-value",
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
			},
		}

		propsJSON, err := json.Marshal(jobProperties)
		require.NoError(t, err, "Failed to marshal job properties")

		createReq := &resource.CreateRequest{
			ResourceType: JobResourceType,
			Properties:   propsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := job.Create(ctx, createReq)
		require.NoError(t, err, "Create should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")
		require.Equal(t, resource.OperationStatusInProgress, createResult.ProgressResult.OperationStatus,
			"Create should return InProgress status")
		require.NotEmpty(t, createResult.ProgressResult.RequestID, "Request ID should not be empty")

		t.Logf("Create initiated with request ID: %s", createResult.ProgressResult.RequestID)

		// Wait for creation to complete
		statusResult, err := testutil.WaitForCreate(t, ctx, job, createResult, testutil.TargetConfig, JobResourceType)
		require.NoError(t, err, "Job creation should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")
		require.NotEmpty(t, statusResult.ProgressResult.NativeID, "Native ID should not be empty")

		nativeID = statusResult.ProgressResult.NativeID
		t.Logf("Job created successfully with native ID: %s", nativeID)

		// Note: ResourceProperties are not populated during LRO status checks
		// The Read test will verify the resource properties
	})

	// Test 2: Read Job
	t.Run("ReadJob", func(t *testing.T) {
		require.NotEmpty(t, nativeID, "Native ID must be set from create step")

		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig:       testutil.TargetConfig,
			ResourceType: JobResourceType,
		}

		readResult, err := job.Read(ctx, readReq)
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

		t.Logf("Successfully read job: %s", utils.GetString(props, "name"))
	})

	// Test 3: List Jobs
	t.Run("ListJobs", func(t *testing.T) {
		listReq := &resource.ListRequest{
			ResourceType: JobResourceType,
			TargetConfig:       testutil.TargetConfig,
		}

		listResult, err := job.List(ctx, listReq)
		require.NoError(t, err, "List should not return error")
		require.NotNil(t, listResult, "List result should not be nil")
		require.NotEmpty(t, listResult.NativeIDs, "List should return at least one resource")

		// Verify our job is in the list
		found := false
		for _, id := range listResult.NativeIDs {
			if id == nativeID {
				found = true
				break
			}
		}

		assert.True(t, found, "Created job should be in list results")
		t.Logf("Found %d jobs in list", len(listResult.NativeIDs))
	})

	// Test 4: Read Non-Existent Job (404)
	t.Run("ReadNotFound", func(t *testing.T) {
		nonExistentID := fmt.Sprintf("projects/%s/locations/%s/jobs/does-not-exist-%s",
			testutil.Project, testutil.Region, uuid.New().String()[:8])

		readReq := &resource.ReadRequest{
			NativeID:     nonExistentID,
			TargetConfig:       testutil.TargetConfig,
			ResourceType: JobResourceType,
		}

		readResult, err := job.Read(ctx, readReq)
		require.NoError(t, err, "Read should not return error even for non-existent resource")
		require.NotNil(t, readResult, "Read result should not be nil")

		// Should return NotFound error code
		assert.NotEmpty(t, readResult.ErrorCode, "Should return error code for non-existent resource")
		assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode,
			"Error code should be NotFound")

		t.Logf("Read non-existent job returned expected NotFound error: %s", readResult.ErrorCode)
	})

	// Test 5: Update Job (should fail - not supported)
	t.Run("UpdateNotSupported", func(t *testing.T) {
		require.NotEmpty(t, nativeID, "Native ID must be set from create step")

		updateProperties := map[string]interface{}{
			"name":     jobName,
			"location": testutil.Region,
			"labels": map[string]interface{}{
				"test":    "formae-cloudrun-job",
				"updated": "true",
			},
		}

		propsJSON, err := json.Marshal(updateProperties)
		require.NoError(t, err, "Failed to marshal update properties")

		updateReq := &resource.UpdateRequest{
			NativeID:          nativeID,
			ResourceType:      JobResourceType,
			DesiredProperties: propsJSON,
			TargetConfig:      testutil.TargetConfig,
		}

		updateResult, err := job.Update(ctx, updateReq)
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

	// Test 6: Delete Job
	t.Run("DeleteJob", func(t *testing.T) {
		require.NotEmpty(t, nativeID, "Native ID must be set from create step")

		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig:       testutil.TargetConfig,
			ResourceType: JobResourceType,
		}

		deleteResult, err := job.Delete(ctx, deleteReq)
		require.NoError(t, err, "Delete should not return error")
		require.NotNil(t, deleteResult, "Delete result should not be nil")
		require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")
		require.Equal(t, resource.OperationStatusInProgress, deleteResult.ProgressResult.OperationStatus,
			"Delete should return InProgress status")
		require.NotEmpty(t, deleteResult.ProgressResult.RequestID, "Request ID should not be empty")

		t.Logf("Delete initiated with request ID: %s", deleteResult.ProgressResult.RequestID)

		// Wait for deletion to complete
		_, err = testutil.WaitForDelete(t, ctx, job, deleteResult, testutil.TargetConfig, JobResourceType)
		require.NoError(t, err, "Job deletion should complete successfully")

		t.Logf("Job deleted successfully")
	})

	// Test 7: Verify Job is Deleted (404 after delete)
	t.Run("VerifyDeleted", func(t *testing.T) {
		require.NotEmpty(t, nativeID, "Native ID must be set from create step")

		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig:       testutil.TargetConfig,
			ResourceType: JobResourceType,
		}

		readResult, err := job.Read(ctx, readReq)
		require.NoError(t, err, "Read should not return error")
		require.NotNil(t, readResult, "Read result should not be nil")

		// Should return NotFound error code
		assert.NotEmpty(t, readResult.ErrorCode, "Should return error code for deleted resource")
		assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode,
			"Error code should be NotFound after deletion")

		t.Logf("Verified job is deleted - Read returned NotFound")
	})
}

// TestJobCreateWithComplexConfig tests Cloud Run job creation with advanced configuration
func TestJobCreateWithComplexConfig(t *testing.T) {

	job, err := NewCloudRunProvisioner(testutil.Config, JobResourceType)
	require.NoError(t, err, "Failed to create Cloud Run job provisioner")

	jobName := fmt.Sprintf("formae-test-complex-job-%s", strings.ToLower(uuid.New().String()[:8]))
	ctx := context.Background()
	nativeID := ""

	t.Run("CreateJobWithMultipleTasks", func(t *testing.T) {
		jobProperties := map[string]interface{}{
			"name":     jobName,
			"location": testutil.Region,
			"labels": map[string]interface{}{
				"test": "formae-cloudrun-complex-job",
			},
			"template": map[string]interface{}{
				"taskCount":   3, // Multiple parallel tasks
				"parallelism": 2, // Run 2 tasks at a time
				"template": map[string]interface{}{
					"maxRetries": 2,
					"timeout":    "300s",
					"containers": []map[string]interface{}{
						{
							"image": "us-docker.pkg.dev/cloudrun/container/job",
							"args": []string{
								"--message=hello",
								"--world",
							},
							"env": []map[string]interface{}{
								{
									"name":  "LOG_LEVEL",
									"value": "info",
								},
								{
									"name":  "TASK_INDEX",
									"value": "$(CLOUD_RUN_TASK_INDEX)",
								},
							},
							"resources": map[string]interface{}{
								"limits": map[string]interface{}{
									"cpu":    "2000m",
									"memory": "1Gi",
								},
							},
						},
					},
				},
			},
		}

		propsJSON, err := json.Marshal(jobProperties)
		require.NoError(t, err, "Failed to marshal job properties")

		createReq := &resource.CreateRequest{
			ResourceType: JobResourceType,
			Properties:   propsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := job.Create(ctx, createReq)
		require.NoError(t, err, "Create should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")

		// Wait for creation
		statusResult, err := testutil.WaitForCreate(t, ctx, job, createResult, testutil.TargetConfig, JobResourceType)
		require.NoError(t, err, "Job creation should complete successfully")

		nativeID = statusResult.ProgressResult.NativeID
		t.Logf("Complex job created: %s", nativeID)

		// Note: ResourceProperties are not populated during LRO status checks
		// The job was created successfully with the complex configuration
	})

	// Cleanup
	t.Run("Cleanup", func(t *testing.T) {
		if nativeID == "" {
			t.Skip("No resource to cleanup")
		}

		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig:       testutil.TargetConfig,
			ResourceType: JobResourceType,
		}

		deleteResult, err := job.Delete(ctx, deleteReq)
		require.NoError(t, err, "Delete should not return error")

		_, err = testutil.WaitForDelete(t, ctx, job, deleteResult, testutil.TargetConfig, JobResourceType)
		require.NoError(t, err, "Job deletion should complete successfully")

		t.Logf("Complex job cleaned up successfully")
	})
}

// TestJobCreateWithVolumes tests Cloud Run job creation with volume mounts
func TestJobCreateWithVolumes(t *testing.T) {

	job, err := NewCloudRunProvisioner(testutil.Config, JobResourceType)
	require.NoError(t, err, "Failed to create Cloud Run job provisioner")

	jobName := fmt.Sprintf("formae-test-vol-job-%s", strings.ToLower(uuid.New().String()[:8]))
	ctx := context.Background()
	nativeID := ""

	t.Run("CreateJobWithEmptyDirVolume", func(t *testing.T) {
		jobProperties := map[string]interface{}{
			"name":     jobName,
			"location": testutil.Region,
			"template": map[string]interface{}{
				"taskCount": 1,
				"template": map[string]interface{}{
					"containers": []map[string]interface{}{
						{
							"image": "us-docker.pkg.dev/cloudrun/container/job",
							"volumeMounts": []map[string]interface{}{
								{
									"name":      "cache-volume",
									"mountPath": "/cache",
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
					"volumes": []map[string]interface{}{
						{
							"name": "cache-volume",
							"emptyDir": map[string]interface{}{
								"medium":    "MEMORY",
								"sizeLimit": "128Mi",
							},
						},
					},
				},
			},
		}

		propsJSON, err := json.Marshal(jobProperties)
		require.NoError(t, err, "Failed to marshal job properties")

		createReq := &resource.CreateRequest{
			ResourceType: JobResourceType,
			Properties:   propsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := job.Create(ctx, createReq)
		require.NoError(t, err, "Create should not return error")

		// Wait for creation
		statusResult, err := testutil.WaitForCreate(t, ctx, job, createResult, testutil.TargetConfig, JobResourceType)
		require.NoError(t, err, "Job creation should complete successfully")

		nativeID = statusResult.ProgressResult.NativeID
		t.Logf("Job with volumes created: %s", nativeID)
	})

	// Cleanup
	t.Run("Cleanup", func(t *testing.T) {
		if nativeID == "" {
			t.Skip("No resource to cleanup")
		}

		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig:       testutil.TargetConfig,
			ResourceType: JobResourceType,
		}

		deleteResult, err := job.Delete(ctx, deleteReq)
		require.NoError(t, err, "Delete should not return error")

		_, err = testutil.WaitForDelete(t, ctx, job, deleteResult, testutil.TargetConfig, JobResourceType)
		require.NoError(t, err, "Job deletion should complete successfully")

		t.Logf("Volume job cleaned up successfully")
	})
}
