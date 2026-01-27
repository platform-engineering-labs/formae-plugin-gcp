// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration

package compute

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

// TestDisk_Create_Integration tests creating a GCP Compute Disk
func TestDisk_Create_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	provisioner := newDiskProvisioner(testutil.Config)

	// Generate unique name
	diskName := fmt.Sprintf("formae-test-disk-create-%s", uuid.New().String()[:8])
	t.Logf("Creating test disk: %s", diskName)

	diskProperties := map[string]interface{}{
		"name":        diskName,
		"project":     testutil.Project,
		"zone":        testutil.Zone,
		"description": "Test disk created by Formae integration test",
		"sizeGb":      10,
		"type":        "pd-balanced",
		"labels": map[string]interface{}{
			"environment": "test",
			"created-by":  "formae-integration-test",
		},
	}

	diskPropsJSON, err := json.Marshal(diskProperties)
	require.NoError(t, err, "Failed to marshal disk properties")

	createReq := &resource.CreateRequest{
		ResourceType: DiskResourceType,
		Properties:   diskPropsJSON,
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
				ResourceType: DiskResourceType,
			}
			deleteResult, _ := provisioner.Delete(ctx, deleteReq)
			if deleteResult != nil && deleteResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
				_, _ = testutil.WaitForDelete(t, ctx, provisioner, deleteResult, testutil.TargetConfig, DiskResourceType)
			}
			t.Logf("✓ Cleaned up test disk: %s", diskName)
		}()
	}

	assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation, "Operation should be Create")
	assert.Equal(t, resource.OperationStatusInProgress, createResult.ProgressResult.OperationStatus, "Should be in progress")
	require.NotEmpty(t, createResult.ProgressResult.RequestID, "RequestID should be set")
	require.NotEmpty(t, createResult.ProgressResult.NativeID, "NativeID should be set")

	t.Logf("Disk creation initiated with RequestID: %s", createResult.ProgressResult.RequestID)
	t.Logf("NativeID: %s", createResult.ProgressResult.NativeID)

	// Wait for creation to complete
	statusResult, err := testutil.WaitForCreate(t, ctx, provisioner, createResult, testutil.TargetConfig, DiskResourceType)
	require.NoError(t, err, "Disk creation should complete successfully")
	require.NotNil(t, statusResult, "Status result should not be nil")

	t.Logf("Disk created successfully")
}

// TestDisk_Read_Integration tests reading a GCP Compute Disk
func TestDisk_Read_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	provisioner := newDiskProvisioner(testutil.Config)

	// Create disk first
	diskName := fmt.Sprintf("formae-test-disk-read-%s", uuid.New().String()[:8])
	t.Logf("Creating test disk for read test: %s", diskName)

	diskProperties := map[string]interface{}{
		"name":        diskName,
		"project":     testutil.Project,
		"zone":        testutil.Zone,
		"description": "Test disk for read integration test",
		"sizeGb":      10,
		"type":        "pd-standard",
		"labels": map[string]interface{}{
			"environment": "test",
		},
	}

	diskPropsJSON, err := json.Marshal(diskProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: DiskResourceType,
		Properties:   diskPropsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := provisioner.Create(ctx, createReq)
	require.NoError(t, err)
	require.NotNil(t, createResult)

	nativeID := createResult.ProgressResult.NativeID

	// Cleanup
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: DiskResourceType,
		}
		deleteResult, _ := provisioner.Delete(ctx, deleteReq)
		if deleteResult != nil && deleteResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
			_, _ = testutil.WaitForDelete(t, ctx, provisioner, deleteResult, testutil.TargetConfig, DiskResourceType)
		}
		t.Logf("✓ Cleaned up test disk: %s", diskName)
	}()

	// Wait for creation
	_, err = testutil.WaitForCreate(t, ctx, provisioner, createResult, testutil.TargetConfig, DiskResourceType)
	require.NoError(t, err)

	// Test Read
	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
		ResourceType: DiskResourceType,
	}

	readResult, err := provisioner.Read(ctx, readReq)
	require.NoError(t, err, "Read operation should not return error")
	require.NotNil(t, readResult, "Read result should not be nil")
	require.Empty(t, readResult.ErrorCode, "Read should not have error code")
	require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

	// Verify properties
	var readProps map[string]interface{}
	err = json.Unmarshal([]byte(readResult.Properties), &readProps)
	require.NoError(t, err, "Failed to unmarshal read properties")

	assert.Equal(t, diskName, readProps["name"], "Disk name should match")
	assert.Equal(t, "Test disk for read integration test", readProps["description"], "Description should match")
	assert.NotEmpty(t, readProps["selfLink"], "selfLink should be present")
	assert.NotEmpty(t, readProps["id"], "id should be present")

	// Verify zone is transformed (should be just zone name, not full URL)
	zone, ok := readProps["zone"].(string)
	require.True(t, ok, "zone should be a string")
	assert.Equal(t, testutil.Zone, zone, "Zone should be transformed to short name")

	// Verify labels
	labels, ok := readProps["labels"].(map[string]interface{})
	require.True(t, ok, "labels should be a map")
	assert.Equal(t, "test", labels["environment"], "environment label should match")

	t.Logf("Read disk properties: name=%s, zone=%s, sizeGb=%v", readProps["name"], readProps["zone"], readProps["sizeGb"])
}

// TestDisk_Update_Integration tests updating disk labels via setLabels API
func TestDisk_Update_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	provisioner := newDiskProvisioner(testutil.Config)

	// Create disk first
	diskName := fmt.Sprintf("formae-test-disk-update-%s", uuid.New().String()[:8])
	t.Logf("Creating test disk for update test: %s", diskName)

	diskProperties := map[string]interface{}{
		"name":    diskName,
		"project": testutil.Project,
		"zone":    testutil.Zone,
		"sizeGb":  10,
		"type":    "pd-standard",
		"labels": map[string]interface{}{
			"environment": "test",
			"version":     "v1",
		},
	}

	diskPropsJSON, err := json.Marshal(diskProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: DiskResourceType,
		Properties:   diskPropsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := provisioner.Create(ctx, createReq)
	require.NoError(t, err)

	nativeID := createResult.ProgressResult.NativeID

	// Cleanup
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: DiskResourceType,
		}
		deleteResult, _ := provisioner.Delete(ctx, deleteReq)
		if deleteResult != nil && deleteResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
			_, _ = testutil.WaitForDelete(t, ctx, provisioner, deleteResult, testutil.TargetConfig, DiskResourceType)
		}
		t.Logf("✓ Cleaned up test disk: %s", diskName)
	}()

	// Wait for creation
	_, err = testutil.WaitForCreate(t, ctx, provisioner, createResult, testutil.TargetConfig, DiskResourceType)
	require.NoError(t, err)

	// Update labels
	updatedProperties := map[string]interface{}{
		"name":    diskName,
		"project": testutil.Project,
		"zone":    testutil.Zone,
		"labels": map[string]interface{}{
			"environment": "test",
			"version":     "v2",
			"updated":     "true",
		},
	}

	updatedPropsJSON, err := json.Marshal(updatedProperties)
	require.NoError(t, err)

	updateReq := &resource.UpdateRequest{
		ResourceType:      DiskResourceType,
		DesiredProperties: updatedPropsJSON,
		NativeID:          nativeID,
		TargetConfig:      testutil.TargetConfig,
	}

	updateResult, err := provisioner.Update(ctx, updateReq)
	require.NoError(t, err, "Update should not return error")
	require.NotNil(t, updateResult, "Update result should not be nil")

	assert.Equal(t, resource.OperationUpdate, updateResult.ProgressResult.Operation, "Operation should be Update")
	assert.Equal(t, resource.OperationStatusInProgress, updateResult.ProgressResult.OperationStatus, "Should be in progress")

	t.Logf("Label update initiated with RequestID: %s", updateResult.ProgressResult.RequestID)

	// Wait for update to complete
	statusResult, err := testutil.WaitForUpdate(t, ctx, provisioner, updateResult, testutil.TargetConfig, DiskResourceType)
	require.NoError(t, err, "Label update should complete successfully")
	require.NotNil(t, statusResult, "Status result should not be nil")

	// Verify labels were updated
	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
		ResourceType: DiskResourceType,
	}

	readResult, err := provisioner.Read(ctx, readReq)
	require.NoError(t, err)

	props := utils.MustParseProperties(readResult.Properties)
	labels := utils.GetObject(props, "labels")
	require.NotNil(t, labels, "Labels should be present")

	assert.Equal(t, "v2", labels["version"], "version label should be updated to v2")
	assert.Equal(t, "true", labels["updated"], "updated label should be present")

	t.Logf("Labels updated successfully: %v", labels)
}

// TestDisk_Delete_Integration tests deleting a GCP Compute Disk
func TestDisk_Delete_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	provisioner := newDiskProvisioner(testutil.Config)

	// Create disk first
	diskName := fmt.Sprintf("formae-test-disk-delete-%s", uuid.New().String()[:8])
	t.Logf("Creating test disk for delete test: %s", diskName)

	diskProperties := map[string]interface{}{
		"name":    diskName,
		"project": testutil.Project,
		"zone":    testutil.Zone,
		"sizeGb":  10,
		"type":    "pd-standard",
	}

	diskPropsJSON, err := json.Marshal(diskProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: DiskResourceType,
		Properties:   diskPropsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := provisioner.Create(ctx, createReq)
	require.NoError(t, err)

	nativeID := createResult.ProgressResult.NativeID

	// Wait for creation
	_, err = testutil.WaitForCreate(t, ctx, provisioner, createResult, testutil.TargetConfig, DiskResourceType)
	require.NoError(t, err)

	// Delete disk
	deleteReq := &resource.DeleteRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
		ResourceType: DiskResourceType,
	}

	deleteResult, err := provisioner.Delete(ctx, deleteReq)
	require.NoError(t, err, "Delete operation should not return error")
	require.NotNil(t, deleteResult, "Delete result should not be nil")
	require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")

	assert.Equal(t, resource.OperationDelete, deleteResult.ProgressResult.Operation, "Operation should be Delete")

	if deleteResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
		t.Logf("Disk deletion initiated with RequestID: %s", deleteResult.ProgressResult.RequestID)

		// Wait for deletion to complete
		statusResult, err := testutil.WaitForDelete(t, ctx, provisioner, deleteResult, testutil.TargetConfig, DiskResourceType)
		require.NoError(t, err, "Disk deletion should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")
	}

	// Verify disk no longer exists
	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
		ResourceType: DiskResourceType,
	}

	readResult, err := provisioner.Read(ctx, readReq)
	require.NoError(t, err, "Read should not return error")
	require.NotNil(t, readResult, "Read result should not be nil")
	assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode, "Should return NotFound")

	t.Logf("Disk deleted successfully")
}

// TestDisk_Delete_NotFound_Integration tests deleting a non-existent disk (idempotent)
func TestDisk_Delete_NotFound_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	provisioner := newDiskProvisioner(testutil.Config)

	// Try to delete non-existent disk
	nonExistentID := fmt.Sprintf("projects/%s/zones/%s/disks/nonexistent-disk-%s",
		testutil.Project, testutil.Zone, uuid.New().String()[:8])

	deleteReq := &resource.DeleteRequest{
		NativeID:     nonExistentID,
		TargetConfig: testutil.TargetConfig,
		ResourceType: DiskResourceType,
	}

	deleteResult, err := provisioner.Delete(ctx, deleteReq)
	require.NoError(t, err, "Delete should not return error")
	require.NotNil(t, deleteResult, "Delete result should not be nil")

	// Delete of non-existent resource should succeed (idempotent)
	assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus,
		"Delete of non-existent disk should succeed (idempotent)")

	t.Logf("Delete of non-existent disk correctly succeeded (idempotent)")
}

// TestDisk_List_Integration tests listing GCP Compute Disks
func TestDisk_List_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	provisioner := newDiskProvisioner(testutil.Config)

	// Create a disk to ensure there's at least one
	diskName := fmt.Sprintf("formae-test-disk-list-%s", uuid.New().String()[:8])
	t.Logf("Creating test disk for list test: %s", diskName)

	diskProperties := map[string]interface{}{
		"name":    diskName,
		"project": testutil.Project,
		"zone":    testutil.Zone,
		"sizeGb":  10,
		"type":    "pd-standard",
	}

	diskPropsJSON, err := json.Marshal(diskProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: DiskResourceType,
		Properties:   diskPropsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := provisioner.Create(ctx, createReq)
	require.NoError(t, err)

	nativeID := createResult.ProgressResult.NativeID

	// Cleanup
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: DiskResourceType,
		}
		deleteResult, _ := provisioner.Delete(ctx, deleteReq)
		if deleteResult != nil && deleteResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
			_, _ = testutil.WaitForDelete(t, ctx, provisioner, deleteResult, testutil.TargetConfig, DiskResourceType)
		}
		t.Logf("✓ Cleaned up test disk: %s", diskName)
	}()

	// Wait for creation
	_, err = testutil.WaitForCreate(t, ctx, provisioner, createResult, testutil.TargetConfig, DiskResourceType)
	require.NoError(t, err)

	// List disks
	listReq := &resource.ListRequest{
		ResourceType: DiskResourceType,
		TargetConfig: testutil.TargetConfig,
	}

	listResult, err := provisioner.List(ctx, listReq)
	require.NoError(t, err, "List operation should not return error")
	require.NotNil(t, listResult, "List result should not be nil")
	require.NotNil(t, listResult.NativeIDs, "NativeIDs should not be nil")

	t.Logf("Found %d disks in zone %s", len(listResult.NativeIDs), testutil.Zone)

	// Verify our disk is in the list
	found := false
	for _, id := range listResult.NativeIDs {
		if id == nativeID {
			found = true
			break
		}
	}
	assert.True(t, found, "Created disk should be in the list")
}

// TestDisk_CreateFromImage_Integration tests creating a disk from a source image
func TestDisk_CreateFromImage_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	provisioner := newDiskProvisioner(testutil.Config)

	diskName := fmt.Sprintf("formae-test-disk-img-%s", uuid.New().String()[:8])
	t.Logf("Creating test disk from image: %s", diskName)

	// Create disk from Debian 12 image
	diskProperties := map[string]interface{}{
		"name":        diskName,
		"project":     testutil.Project,
		"zone":        testutil.Zone,
		"description": "Boot disk created from Debian 12 image",
		"sizeGb":      20,
		"type":        "pd-balanced",
		"sourceImage": "projects/debian-cloud/global/images/family/debian-12",
		"labels": map[string]interface{}{
			"environment": "test",
			"os":          "debian-12",
		},
	}

	diskPropsJSON, err := json.Marshal(diskProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: DiskResourceType,
		Properties:   diskPropsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := provisioner.Create(ctx, createReq)
	require.NoError(t, err, "Create operation should not return error")
	require.NotNil(t, createResult)

	nativeID := createResult.ProgressResult.NativeID
	t.Logf("Disk creation initiated: %s", nativeID)

	// Cleanup
	defer func() {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: DiskResourceType,
		}
		deleteResult, _ := provisioner.Delete(ctx, deleteReq)
		if deleteResult != nil && deleteResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
			_, _ = testutil.WaitForDelete(t, ctx, provisioner, deleteResult, testutil.TargetConfig, DiskResourceType)
		}
		t.Logf("✓ Cleaned up test disk: %s", diskName)
	}()

	// Wait for creation
	_, err = testutil.WaitForCreate(t, ctx, provisioner, createResult, testutil.TargetConfig, DiskResourceType)
	require.NoError(t, err, "Disk creation should complete successfully")

	// Verify disk properties
	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
		ResourceType: DiskResourceType,
	}

	readResult, err := provisioner.Read(ctx, readReq)
	require.NoError(t, err)

	props := utils.MustParseProperties(readResult.Properties)

	// Verify sourceImage is set
	sourceImage := utils.GetString(props, "sourceImage")
	assert.Contains(t, sourceImage, "debian", "Source image should contain debian")
	t.Logf("Disk created from image: %s", sourceImage)

	// Verify guest OS features are present (populated from source image)
	guestOsFeatures := props["guestOsFeatures"]
	assert.NotNil(t, guestOsFeatures, "Guest OS features should be populated from source image")
	t.Logf("Guest OS features: %v", guestOsFeatures)
}
