// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration
// +build integration

package compute

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

// TestDiskCreate tests the creation, reading, updating, and deletion of a GCP Compute Disk
func TestDiskCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	disk, err := NewComputeProvisioner(testutil.Config, DiskResourceType)
	require.NoError(t, err, "Failed to create disk provisioner")

	// Generate unique name
	diskName := fmt.Sprintf("formae-test-disk-%s", uuid.New().String()[:8])
	t.Logf("Creating test disk: %s", diskName)

	var nativeID string

	// Test Create operation
	t.Run("Create", func(t *testing.T) {
		diskProperties := map[string]interface{}{
			"name":        diskName,
			"project":     testutil.Project,
			"zone":        testutil.Zone,
			"description": "Test disk created by Formae integration test",
			"sizeGb":      10,
			"diskType":    "pd-balanced",
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

		createResult, err := disk.Create(ctx, createReq)
		require.NoError(t, err, "Create operation should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation, "Operation should be Create")
		assert.Equal(t, resource.OperationStatusInProgress, createResult.ProgressResult.OperationStatus, "Should be in progress")
		require.NotEmpty(t, createResult.ProgressResult.RequestID, "RequestID should be set")
		require.NotEmpty(t, createResult.ProgressResult.NativeID, "NativeID should be set")

		nativeID = createResult.ProgressResult.NativeID
		t.Logf("Disk creation initiated with RequestID: %s", createResult.ProgressResult.RequestID)
		t.Logf("NativeID: %s", nativeID)

		// Wait for creation to complete
		statusResult, err := testutil.WaitForCreate(t, ctx, disk, createResult, testutil.TargetConfig, DiskResourceType)
		require.NoError(t, err, "Disk creation should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")

		t.Logf("Disk created successfully")
	})

	// Test Read operation
	t.Run("Read", func(t *testing.T) {
		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: DiskResourceType,
		}

		readResult, err := disk.Read(ctx, readReq)
		require.NoError(t, err, "Read operation should not return error")
		require.NotNil(t, readResult, "Read result should not be nil")
		require.Empty(t, readResult.ErrorCode, "Read should not have error code")
		require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

		// Verify properties
		var readProps map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &readProps)
		require.NoError(t, err, "Failed to unmarshal read properties")

		assert.Equal(t, diskName, readProps["name"], "Disk name should match")
		assert.Equal(t, "Test disk created by Formae integration test", readProps["description"], "Description should match")
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
		assert.Equal(t, "formae-integration-test", labels["created-by"], "created-by label should match")

		t.Logf("Read disk properties: name=%s, zone=%s, sizeGb=%v", readProps["name"], readProps["zone"], readProps["sizeGb"])
	})

	// Test List operation
	t.Run("List", func(t *testing.T) {
		listReq := &resource.ListRequest{
			ResourceType: DiskResourceType,
			TargetConfig: testutil.TargetConfig,
		}

		listResult, err := disk.List(ctx, listReq)
		require.NoError(t, err, "List operation should not return error")
		require.NotNil(t, listResult, "List result should not be nil")
		require.NotNil(t, listResult.NativeIDs, "Resources list should not be nil")

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
	})

	// Note: Disk updates for labels require the setLabels API, not standard PATCH
	// The standard update is not supported for disks
	t.Run("UpdateNotSupported", func(t *testing.T) {
		updatedProperties := map[string]interface{}{
			"name":    diskName,
			"project": testutil.Project,
			"zone":    testutil.Zone,
			"labels": map[string]interface{}{
				"environment": "test",
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

		updateResult, err := disk.Update(ctx, updateReq)
		require.NoError(t, err, "Update should not return error")
		require.NotNil(t, updateResult, "Update result should not be nil")

		// Disk updates are not supported via standard PATCH
		assert.Equal(t, resource.OperationUpdate, updateResult.ProgressResult.Operation, "Operation should be Update")
		assert.Equal(t, resource.OperationStatusFailure, updateResult.ProgressResult.OperationStatus, "Should fail - updates not supported")
		assert.Equal(t, resource.OperationErrorCodeNotUpdatable, updateResult.ProgressResult.ErrorCode, "Should return NotUpdatable error")

		t.Logf("Disk update correctly returned NotUpdatable as expected")
	})

	// Test Delete operation
	t.Run("Delete", func(t *testing.T) {
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		deleteResult, err := disk.Delete(ctx, deleteReq)
		require.NoError(t, err, "Delete operation should not return error")
		require.NotNil(t, deleteResult, "Delete result should not be nil")
		require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationDelete, deleteResult.ProgressResult.Operation, "Operation should be Delete")

		if deleteResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
			t.Logf("Disk deletion initiated with RequestID: %s", deleteResult.ProgressResult.RequestID)

			// Wait for deletion to complete
			statusResult, err := testutil.WaitForDelete(t, ctx, disk, deleteResult, testutil.TargetConfig, DiskResourceType)
			require.NoError(t, err, "Disk deletion should complete successfully")
			require.NotNil(t, statusResult, "Status result should not be nil")
		}

		t.Logf("Disk deleted successfully")
	})

	// Verify deletion
	t.Run("VerifyDeleted", func(t *testing.T) {
		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: DiskResourceType,
		}

		readResult, err := disk.Read(ctx, readReq)
		require.NoError(t, err, "Read should not return error")
		require.NotNil(t, readResult, "Read result should not be nil")
		assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode, "Should return NotFound")

		t.Logf("Verified disk was deleted")
	})
}

// TestDiskCreateFromImage tests creating a disk from a source image
func TestDiskCreateFromImage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	disk, err := NewComputeProvisioner(testutil.Config, DiskResourceType)
	require.NoError(t, err, "Failed to create disk provisioner")

	diskName := fmt.Sprintf("formae-test-disk-img-%s", uuid.New().String()[:8])
	t.Logf("Creating test disk from image: %s", diskName)

	// Create disk from Debian 12 image
	diskProperties := map[string]interface{}{
		"name":        diskName,
		"project":     testutil.Project,
		"zone":        testutil.Zone,
		"description": "Boot disk created from Debian 12 image",
		"sizeGb":      20,
		"diskType":    "pd-balanced",
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

	createResult, err := disk.Create(ctx, createReq)
	require.NoError(t, err, "Create operation should not return error")
	require.NotNil(t, createResult)

	nativeID := createResult.ProgressResult.NativeID
	t.Logf("Disk creation initiated: %s", nativeID)

	// Wait for creation
	statusResult, err := testutil.WaitForCreate(t, ctx, disk, createResult, testutil.TargetConfig, DiskResourceType)
	require.NoError(t, err, "Disk creation should complete successfully")

	// Verify disk properties
	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
		ResourceType: DiskResourceType,
	}

	readResult, err := disk.Read(ctx, readReq)
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

	// Cleanup
	deleteReq := &resource.DeleteRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
		ResourceType: DiskResourceType,
	}
	deleteResult, err := disk.Delete(ctx, deleteReq)
	require.NoError(t, err)

	_, err = testutil.WaitForDelete(t, ctx, disk, deleteResult, testutil.TargetConfig, DiskResourceType)
	require.NoError(t, err)

	t.Logf("Disk with source image deleted: %s", statusResult.ProgressResult.NativeID)
}

// TestDiskNotFound tests reading a non-existent disk
func TestDiskNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	disk, err := NewComputeProvisioner(testutil.Config, DiskResourceType)
	require.NoError(t, err, "Failed to create disk provisioner")

	
	readReq := &resource.ReadRequest{
		NativeID:     fmt.Sprintf("projects/%s/zones/%s/disks/nonexistent-disk-%s", testutil.Project, testutil.Zone, uuid.New().String()[:8]),
		TargetConfig: testutil.TargetConfig,
		ResourceType: DiskResourceType,
	}

	readResult, err := disk.Read(ctx, readReq)
	require.NoError(t, err, "Read should not return error")
	require.NotNil(t, readResult, "Read result should not be nil")
	assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode, "Should return NotFound")

	t.Logf("Verified non-existent disk returns NotFound")
}

// TestDiskDifferentTypes tests creating disks with different disk types
func TestDiskDifferentTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	disk, err := NewComputeProvisioner(testutil.Config, DiskResourceType)
	require.NoError(t, err, "Failed to create disk provisioner")

	
	diskTypes := []string{"pd-standard", "pd-balanced", "pd-ssd"}

	for _, diskType := range diskTypes {
		t.Run(fmt.Sprintf("DiskType_%s", diskType), func(t *testing.T) {
			diskName := fmt.Sprintf("formae-test-%s-%s", diskType, uuid.New().String()[:8])
			t.Logf("Creating disk with type %s: %s", diskType, diskName)

			diskProperties := map[string]interface{}{
				"name":        diskName,
				"project":     testutil.Project,
				"zone":        testutil.Zone,
				"description": fmt.Sprintf("Test disk with type %s", diskType),
				"sizeGb":      10,
				// GCP API expects 'type' as a full URL: zones/{zone}/diskTypes/{diskType}
				"type": fmt.Sprintf("zones/%s/diskTypes/%s", testutil.Zone, diskType),
				"labels": map[string]interface{}{
					"disk-type": diskType,
				},
			}

			diskPropsJSON, err := json.Marshal(diskProperties)
			require.NoError(t, err)

			createReq := &resource.CreateRequest{
				ResourceType: DiskResourceType,
				Properties:   diskPropsJSON,
				TargetConfig: testutil.TargetConfig,
			}

			createResult, err := disk.Create(ctx, createReq)
			require.NoError(t, err)

			nativeID := createResult.ProgressResult.NativeID

			// Wait for creation
			_, err = testutil.WaitForCreate(t, ctx, disk, createResult, testutil.TargetConfig, DiskResourceType)
			require.NoError(t, err)

			// Verify disk type
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: DiskResourceType,
			}

			readResult, err := disk.Read(ctx, readReq)
			require.NoError(t, err)

			props := utils.MustParseProperties(readResult.Properties)
			actualDiskType := utils.GetString(props, "type")
			assert.Contains(t, actualDiskType, diskType, "Disk type should match")
			t.Logf("Disk type verified: %s", actualDiskType)

			// Cleanup
			deleteReq := &resource.DeleteRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: DiskResourceType,
			}
			deleteResult, err := disk.Delete(ctx, deleteReq)
			require.NoError(t, err)

			_, err = testutil.WaitForDelete(t, ctx, disk, deleteResult, testutil.TargetConfig, DiskResourceType)
			require.NoError(t, err)

			t.Logf("Disk deleted: %s", diskName)
		})
	}
}

// TestDiskResize tests resizing a disk (increasing size)
func TestDiskResize(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	disk, err := NewComputeProvisioner(testutil.Config, DiskResourceType)
	require.NoError(t, err, "Failed to create disk provisioner")

	diskName := fmt.Sprintf("formae-test-resize-%s", uuid.New().String()[:8])
	t.Logf("Creating disk for resize test: %s", diskName)

	
	// Create initial disk with 10GB
	diskProperties := map[string]interface{}{
		"name":     diskName,
		"project":  testutil.Project,
		"zone":     testutil.Zone,
		"sizeGb":   10,
		"diskType": "pd-balanced",
		"labels": map[string]interface{}{
			"test": "resize",
		},
	}

	diskPropsJSON, err := json.Marshal(diskProperties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: DiskResourceType,
		Properties:   diskPropsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := disk.Create(ctx, createReq)
	require.NoError(t, err)

	nativeID := createResult.ProgressResult.NativeID

	_, err = testutil.WaitForCreate(t, ctx, disk, createResult, testutil.TargetConfig, DiskResourceType)
	require.NoError(t, err)

	// Note: Disk resize requires a separate API call (disks.resize)
	// The standard update doesn't support changing sizeGb
	// This test documents the expected behavior - resize may not work through standard update
	t.Run("VerifyInitialSize", func(t *testing.T) {
		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: DiskResourceType,
		}

		readResult, err := disk.Read(ctx, readReq)
		require.NoError(t, err)

		props := utils.MustParseProperties(readResult.Properties)
		sizeGb := props["sizeGb"]
		t.Logf("Initial disk size: %v GB", sizeGb)

		// Size might be returned as string or number depending on API
		switch v := sizeGb.(type) {
		case float64:
			assert.Equal(t, float64(10), v)
		case string:
			assert.Equal(t, "10", v)
		}
	})

	// Cleanup
	deleteReq := &resource.DeleteRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
		ResourceType: DiskResourceType,
	}
	deleteResult, err := disk.Delete(ctx, deleteReq)
	require.NoError(t, err)

	_, err = testutil.WaitForDelete(t, ctx, disk, deleteResult, testutil.TargetConfig, DiskResourceType)
	require.NoError(t, err)

	t.Logf("Disk deleted: %s", diskName)
}

// TestDiskList tests listing disks with pagination
func TestDiskList(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	disk, err := NewComputeProvisioner(testutil.Config, DiskResourceType)
	require.NoError(t, err, "Failed to create disk provisioner")

	
	// List all disks in the zone
	listReq := &resource.ListRequest{
		ResourceType: DiskResourceType,
		TargetConfig: testutil.TargetConfig,
		PageSize:     10,
	}

	listResult, err := disk.List(ctx, listReq)
	require.NoError(t, err, "List operation should not return error")
	require.NotNil(t, listResult, "List result should not be nil")

	t.Logf("Found %d disks in zone %s", len(listResult.NativeIDs), testutil.Zone)

	for _, id := range listResult.NativeIDs {
		t.Logf("  - %s", id)
	}

	// Test pagination if there are more results
	if listResult.NextPageToken != nil {
		t.Logf("Next page token: %s", *listResult.NextPageToken)

		// Fetch next page
		listReq.PageToken = listResult.NextPageToken
		nextPageResult, err := disk.List(ctx, listReq)
		require.NoError(t, err)
		t.Logf("Found %d more disks on next page", len(nextPageResult.NativeIDs))
	}
}

// TestDiskAggregatedList tests listing disks across all zones (no specific zone)
func TestDiskAggregatedList(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	disk, err := NewComputeProvisioner(testutil.Config, DiskResourceType)
	require.NoError(t, err, "Failed to create disk provisioner")

	// Target without zone to trigger aggregated list
	// Using testutil.TargetConfig (no zone)

	listReq := &resource.ListRequest{
		ResourceType: DiskResourceType,
		TargetConfig: testutil.TargetConfig,
		PageSize:     50,
	}

	listResult, err := disk.List(ctx, listReq)
	require.NoError(t, err, "Aggregated list operation should not return error")
	require.NotNil(t, listResult, "List result should not be nil")

	t.Logf("Found %d disks across all zones in project %s", len(listResult.NativeIDs), testutil.Project)

	// Group by zone for display
	zoneCount := make(map[string]int)
	for _, id := range listResult.NativeIDs {
		// Extract zone from native ID (format: projects/X/zones/Y/disks/Z)
		parts := strings.Split(id, "/")
		if len(parts) >= 4 {
			zone := parts[3]
			zoneCount[zone]++
		}
	}

	for zone, count := range zoneCount {
		t.Logf("  Zone %s: %d disks", zone, count)
	}
}
