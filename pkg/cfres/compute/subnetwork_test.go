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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/testutil"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSubnetworkCreate tests the creation, reading, and deletion of a GCP Subnetwork
func TestSubnetworkCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create provisioner instances
	network, err := NewComputeProvisioner(testutil.Config, NetworkResourceType)
	require.NoError(t, err, "Failed to create network provisioner")
	subnetwork, err := NewComputeProvisioner(testutil.Config, SubnetworkResourceType)
	require.NoError(t, err, "Failed to create subnetwork provisioner")

	// Generate unique names
	networkName := fmt.Sprintf("formae-test-net-%s", uuid.New().String()[:8])
	subnetworkName := fmt.Sprintf("formae-test-subnet-%s", uuid.New().String()[:8])
	t.Logf("Creating test network: %s", networkName)
	t.Logf("Creating test subnetwork: %s", subnetworkName)

	ctx := context.Background()
	networkSelfLink := ""
	networkNativeID := ""

	// First, create a network for the subnetwork
	t.Run("SetupNetwork", func(t *testing.T) {
		networkProperties := map[string]interface{}{
			"name":                  networkName,
			"description":           "Test network for subnetwork integration test",
			"autoCreateSubnetworks": false,
			"routingConfig": map[string]interface{}{
				"routingMode": "REGIONAL",
			},
		}

		networkPropsJSON, err := json.Marshal(networkProperties)
		require.NoError(t, err, "Failed to marshal network properties")

		createNetworkReq := &resource.CreateRequest{
			ResourceType: "GCP::Compute::Network",
			Properties:   networkPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := network.Create(ctx, createNetworkReq)
		require.NoError(t, err, "Network create should not return error")
		require.NotNil(t, createResult, "Network create result should not be nil")

		// Wait for network creation to complete
		statusResult, err := testutil.WaitForCreate(t, ctx, network, createResult, testutil.TargetConfig, "GCP::Compute::Network")
		require.NoError(t, err, "Network creation should complete successfully")
		networkNativeID = statusResult.ProgressResult.NativeID
		t.Logf("Network created with native ID: %s", networkNativeID)
		var networkProps map[string]interface{}
		err = json.Unmarshal(statusResult.ProgressResult.ResourceProperties, &networkProps)
		require.NoError(t, err, "Failed to unmarshal network properties")
		networkSelfLink = utils.GetString(networkProps, "selfLink")
	})

	// Test Subnetwork Create operation
	t.Run("CreateSubnetwork", func(t *testing.T) {
		//networkSelfLink = "https://www.googleapis.com/compute/v1/projects/development-477117/global/networks/formae-test-net-39c5c940"
		subnetworkProperties := map[string]interface{}{
			"name":                    subnetworkName,
			"description":             "Test subnetwork created by Formae integration test",
			"ipCidrRange":             "10.0.1.0/24",
			"network":                 networkSelfLink,
			"privateIpGoogleAccess":   true,
			"privateIpv6GoogleAccess": "DISABLE_GOOGLE_ACCESS",
		}

		subnetworkPropsJSON, err := json.Marshal(subnetworkProperties)
		require.NoError(t, err, "Failed to marshal subnetwork properties")

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Compute::Subnetwork",
			Properties:   subnetworkPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		// Create the subnetwork
		createResult, err := subnetwork.Create(ctx, createReq)
		require.NoError(t, err, "Create operation should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation, "Operation should be Create")
		assert.Equal(t, resource.OperationStatusInProgress, createResult.ProgressResult.OperationStatus, "Should be in progress")
		require.NotEmpty(t, createResult.ProgressResult.RequestID, "RequestID should be set")
		require.NotEmpty(t, createResult.ProgressResult.NativeID, "NativeID should be set")

		t.Logf("Subnetwork creation initiated with RequestID: %s", createResult.ProgressResult.RequestID)

		// Wait for creation to complete
		statusResult, err := testutil.WaitForCreate(t, ctx, subnetwork, createResult, testutil.TargetConfig, "GCP::Compute::Subnetwork")
		require.NoError(t, err, "Subnetwork creation should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")

		nativeID := statusResult.ProgressResult.NativeID
		t.Logf("Subnetwork created with native ID: %s", nativeID)

		// Test Read operation
		t.Run("Read", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Compute::Subnetwork",
			}

			readResult, err := subnetwork.Read(ctx, readReq)
			require.NoError(t, err, "Read operation should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			require.Empty(t, readResult.ErrorCode, "Read should not have error code")
			require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

			// Verify properties
			var readProps map[string]interface{}
			err = json.Unmarshal([]byte(readResult.Properties), &readProps)
			require.NoError(t, err, "Failed to unmarshal read properties")

			assert.Equal(t, subnetworkName, readProps["name"], "Subnetwork name should match")
			assert.Equal(t, "10.0.1.0/24", readProps["ipCidrRange"], "IP CIDR range should match")
			assert.Equal(t, true, readProps["privateIpGoogleAccess"], "Private IP Google Access should be true")

			t.Logf("Read subnetwork properties: %+v", readProps)
		})

		// Test List operation
		t.Run("List", func(t *testing.T) {
			listReq := &resource.ListRequest{
				ResourceType: "GCP::Compute::Subnetwork",
				TargetConfig: testutil.TargetConfig,
			}

			listResult, err := subnetwork.List(ctx, listReq)
			require.NoError(t, err, "List operation should not return error")
			require.NotNil(t, listResult, "List result should not be nil")
			require.NotNil(t, listResult.NativeIDs, "Resources list should not be nil")

			t.Logf("Found %d subnetworks in region %s", len(listResult.NativeIDs), testutil.Region)
		})

		// Test Delete operation
		t.Run("Delete", func(t *testing.T) {
			deleteReq := &resource.DeleteRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Compute::Subnetwork",
			}

			deleteResult, err := subnetwork.Delete(ctx, deleteReq)
			require.NoError(t, err, "Delete operation should not return error")
			require.NotNil(t, deleteResult, "Delete result should not be nil")
			require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")

			assert.Equal(t, resource.OperationDelete, deleteResult.ProgressResult.Operation, "Operation should be Delete")
			assert.Equal(t, resource.OperationStatusInProgress, deleteResult.ProgressResult.OperationStatus, "Should be in progress")
			require.NotEmpty(t, deleteResult.ProgressResult.RequestID, "RequestID should be set")

			t.Logf("Subnetwork deletion initiated with RequestID: %s", deleteResult.ProgressResult.RequestID)

			// Wait for deletion to complete
			statusResult, err := testutil.WaitForDelete(t, ctx, subnetwork, deleteResult, testutil.TargetConfig, "GCP::Compute::Subnetwork")
			require.NoError(t, err, "Subnetwork deletion should complete successfully")
			require.NotNil(t, statusResult, "Status result should not be nil")

			t.Logf("Subnetwork deleted successfully")
		})

		// Verify deletion
		t.Run("VerifyDeleted", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Compute::Subnetwork",
			}
			time.Sleep(5 * time.Second) // Wait a bit to ensure deletion is fully propagated
			readResult, err := subnetwork.Read(ctx, readReq)
			require.NoError(t, err, "Read should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode, "Should return NotFound")

			t.Logf("Verified subnetwork was deleted")
		})
	})

	// Cleanup: Delete the network
	t.Run("CleanupNetwork", func(t *testing.T) {
		deleteReq := &resource.DeleteRequest{
			NativeID:     networkNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Compute::Network",
		}

		deleteResult, err := network.Delete(ctx, deleteReq)
		require.NoError(t, err, "Network delete should not return error")

		// Wait for network deletion to complete
		_, err = testutil.WaitForDelete(t, ctx, network, deleteResult, testutil.TargetConfig, "GCP::Compute::Network")
		require.NoError(t, err, "Network deletion should complete successfully")

		t.Logf("Network deleted: %s", networkNativeID)
	})
}

// TestSubnetworkWithMultipleSubnets tests creating multiple subnets in the same network
func TestSubnetworkWithMultipleSubnets(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	network, err := NewComputeProvisioner(testutil.Config, NetworkResourceType)
	require.NoError(t, err, "Failed to create network provisioner")
	subnetwork, err := NewComputeProvisioner(testutil.Config, SubnetworkResourceType)
	require.NoError(t, err, "Failed to create subnetwork provisioner")

	networkName := fmt.Sprintf("formae-test-multisubnet-%s", uuid.New().String()[:8])
	t.Logf("Creating test network with multiple subnets: %s", networkName)

	ctx := context.Background()

	// Create network
	networkProperties := map[string]interface{}{
		"name":                  networkName,
		"autoCreateSubnetworks": false,
		"routingConfig": map[string]interface{}{
			"routingMode": "REGIONAL",
		},
	}

	networkPropsJSON, err := json.Marshal(networkProperties)
	require.NoError(t, err)

	createNetworkReq := &resource.CreateRequest{
		ResourceType: "GCP::Compute::Network",
		Properties:   networkPropsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := network.Create(ctx, createNetworkReq)
	require.NoError(t, err)

	networkCreateStatus, err := testutil.WaitForCreate(t, ctx, network, createResult, testutil.TargetConfig, "GCP::Compute::Network")
	require.NoError(t, err)

	var networkProps map[string]interface{}
	err = json.Unmarshal(networkCreateStatus.ProgressResult.ResourceProperties, &networkProps)
	require.NoError(t, err, "Failed to unmarshal network properties")
	networkSelfLink := utils.GetString(networkProps, "selfLink")

	// Create multiple subnets
	subnetConfigs := []struct {
		name      string
		cidr      string
		enablePIA bool
	}{
		{"public-subnet", "10.0.1.0/24", false},
		{"private-subnet", "10.0.2.0/24", true},
		{"dmz-subnet", "10.0.3.0/24", false},
	}

	createdSubnets := make([]string, 0, len(subnetConfigs))

	for _, subnetCfg := range subnetConfigs {
		subnetName := fmt.Sprintf("formae-test-%s-%s", subnetCfg.name, uuid.New().String()[:8])

		t.Run(fmt.Sprintf("Create_%s", subnetCfg.name), func(t *testing.T) {
			subnetProperties := map[string]interface{}{
				"name":                  subnetName,
				"ipCidrRange":           subnetCfg.cidr,
				"network":               networkSelfLink,
				"privateIpGoogleAccess": subnetCfg.enablePIA,
			}

			subnetPropsJSON, err := json.Marshal(subnetProperties)
			require.NoError(t, err)

			createReq := &resource.CreateRequest{
				ResourceType: "GCP::Compute::Subnetwork",
				Properties:   subnetPropsJSON,
				TargetConfig: testutil.TargetConfig,
			}

			createResult, err := subnetwork.Create(ctx, createReq)
			require.NoError(t, err)

			statusResult, err := testutil.WaitForCreate(t, ctx, subnetwork, createResult, testutil.TargetConfig, "GCP::Compute::Subnetwork")
			require.NoError(t, err)

			createdSubnets = append(createdSubnets, statusResult.ProgressResult.NativeID)
			t.Logf("Created subnet %s with CIDR %s", subnetName, subnetCfg.cidr)
		})
	}

	// Cleanup all subnets
	for _, subnetID := range createdSubnets {
		deleteReq := &resource.DeleteRequest{
			NativeID:     subnetID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Compute::Subnetwork",
		}

		deleteResult, err := subnetwork.Delete(ctx, deleteReq)
		require.NoError(t, err)

		_, err = testutil.WaitForDelete(t, ctx, subnetwork, deleteResult, testutil.TargetConfig, "GCP::Compute::Subnetwork")
		require.NoError(t, err)
	}

	// Cleanup network
	networkNativeID := networkCreateStatus.ProgressResult.NativeID
	deleteNetworkReq := &resource.DeleteRequest{
		NativeID:     networkNativeID,
		TargetConfig: testutil.TargetConfig,
		ResourceType: "GCP::Compute::Network",
	}

	deleteResult, err := network.Delete(ctx, deleteNetworkReq)
	require.NoError(t, err)

	_, err = testutil.WaitForDelete(t, ctx, network, deleteResult, testutil.TargetConfig, "GCP::Compute::Network")
	require.NoError(t, err)
}

// TestSubnetworkUpdate tests updating a GCP Subnetwork
func TestSubnetworkUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	subnetwork, err := NewComputeProvisioner(testutil.Config, SubnetworkResourceType)
	require.NoError(t, err, "Failed to create subnetwork provisioner")
	subnetName := fmt.Sprintf("formae-test-subnet-update-%s", uuid.New().String()[:8])
	networkName := fmt.Sprintf("formae-test-net-update-%s", uuid.New().String()[:8])
	t.Logf("Creating test subnetwork for update: %s", subnetName)

	ctx := context.Background()

	// First create a network
	network, err := NewComputeProvisioner(testutil.Config, NetworkResourceType)
	require.NoError(t, err, "Failed to create network provisioner")
	networkProps := map[string]interface{}{
		"name":                  networkName,
		"autoCreateSubnetworks": false,
	}
	networkPropsJSON, _ := json.Marshal(networkProps)

	netCreateReq := &resource.CreateRequest{
		ResourceType: "GCP::Compute::Network",
		Properties:   networkPropsJSON,
		TargetConfig: testutil.TargetConfig,
	}

	netCreateResult, _ := network.Create(ctx, netCreateReq)
	netStatus, _ := testutil.WaitForCreate(t, ctx, network, netCreateResult, testutil.TargetConfig, "GCP::Compute::Network")
	networkID := netStatus.ProgressResult.NativeID

	// Create subnetwork
	properties := map[string]interface{}{
		"name":                  subnetName,
		"network":               fmt.Sprintf("projects/%s/global/networks/%s", testutil.Project, networkName),
		"ipCidrRange":           "10.0.0.0/24",
		"privateIpGoogleAccess": false,
		"enableFlowLogs":        false,
	}

	propertiesJSON, err := json.Marshal(properties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: "GCP::Compute::Subnetwork",
		Properties:   propertiesJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := subnetwork.Create(ctx, createReq)
	require.NoError(t, err)

	statusResult, err := testutil.WaitForCreate(t, ctx, subnetwork, createResult, testutil.TargetConfig, "GCP::Compute::Subnetwork")
	require.NoError(t, err)
	nativeID := statusResult.ProgressResult.NativeID

	// Update subnetwork
	t.Run("Update", func(t *testing.T) {
		// Add a secondary IP range (this is a mutable field)
		updatedProperties := map[string]interface{}{
			"secondaryIpRanges": []map[string]interface{}{
				{
					"rangeName":   "secondary-range-1",
					"ipCidrRange": "192.168.1.0/24",
				},
			},
		}

		updatedPropsJSON, err := json.Marshal(updatedProperties)
		require.NoError(t, err)

		updateReq := &resource.UpdateRequest{
			ResourceType:      "GCP::Compute::Subnetwork",
			DesiredProperties: updatedPropsJSON,
			NativeID:          nativeID,
			TargetConfig:      testutil.TargetConfig,
		}

		updateResult, err := subnetwork.Update(ctx, updateReq)
		require.NoError(t, err)
		require.NotNil(t, updateResult)
		assert.Equal(t, resource.OperationUpdate, updateResult.ProgressResult.Operation)
		assert.Equal(t, resource.OperationStatusInProgress, updateResult.ProgressResult.OperationStatus,
			"Update should return InProgress status, not Failed or Success")

		// Wait for update to complete
		if updateResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
			statusReq := &resource.StatusRequest{
				RequestID:    updateResult.ProgressResult.RequestID,
				ResourceType: "GCP::Compute::Subnetwork",
				TargetConfig: testutil.TargetConfig,
			}

			statusResult, err := testutil.WaitForStatus(t, ctx, subnetwork, statusReq)
			require.NoError(t, err)
			assert.Equal(t, resource.OperationStatusSuccess, statusResult.ProgressResult.OperationStatus)
		}

		// Verify update
		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			ResourceType: "GCP::Compute::Subnetwork",
			TargetConfig: testutil.TargetConfig,
		}

		readResult, err := subnetwork.Read(ctx, readReq)
		require.NoError(t, err)

		var readProps map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &readProps)
		require.NoError(t, err)

		// Verify the secondary IP range was added
		if secondaryRanges, ok := readProps["secondaryIpRanges"].([]interface{}); ok {
			assert.Len(t, secondaryRanges, 1, "Should have 1 secondary IP range")
			if len(secondaryRanges) > 0 {
				if rangeMap, ok := secondaryRanges[0].(map[string]interface{}); ok {
					assert.Equal(t, "secondary-range-1", rangeMap["rangeName"], "Range name should match")
					assert.Equal(t, "192.168.1.0/24", rangeMap["ipCidrRange"], "CIDR range should match")
				}
			}
		} else {
			t.Error("secondaryIpRanges should be present in response")
		}

		t.Logf("Subnetwork updated successfully with secondary IP range")
	})

	// Cleanup
	deleteReq := &resource.DeleteRequest{
		NativeID:     nativeID,
		ResourceType: "GCP::Compute::Subnetwork",
		TargetConfig: testutil.TargetConfig,
	}
	delResult, _ := subnetwork.Delete(ctx, deleteReq)
	testutil.WaitForDelete(t, ctx, subnetwork, delResult, testutil.TargetConfig, "GCP::Compute::Subnetwork")

	// Delete network
	netDeleteReq := &resource.DeleteRequest{
		NativeID:     networkID,
		ResourceType: "GCP::Compute::Network",
		TargetConfig: testutil.TargetConfig,
	}
	netDelResult, _ := network.Delete(ctx, netDeleteReq)
	testutil.WaitForDelete(t, ctx, network, netDelResult, testutil.TargetConfig, "GCP::Compute::Network")
}

// TestSubnetworkList tests listing GCP Subnetworks
func TestSubnetworkList(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	subnetwork, err := NewComputeProvisioner(testutil.Config, SubnetworkResourceType)
	require.NoError(t, err, "Failed to create subnetwork provisioner")

	ctx := context.Background()

	t.Run("ListSubnetworks", func(t *testing.T) {
		listReq := &resource.ListRequest{
			ResourceType: "GCP::Compute::Subnetwork",
			TargetConfig: testutil.TargetConfig,
		}

		listResult, err := subnetwork.List(ctx, listReq)
		require.NoError(t, err, "List operation should not return error")
		require.NotNil(t, listResult, "List result should not be nil")
		require.NotNil(t, listResult.NativeIDs, "NativeIDs list should not be nil")

		t.Logf("Found %d subnetworks in region %s", len(listResult.NativeIDs), testutil.Region)

		// Log first few subnetworks for debugging
		for i, nativeID := range listResult.NativeIDs {
			if i >= 5 {
				t.Logf("  ... and %d more", len(listResult.NativeIDs)-5)
				break
			}
			t.Logf("  - %s", nativeID)
		}
	})

	t.Run("ListWithPagination", func(t *testing.T) {
		// Test pagination if there are enough subnetworks
		listReq := &resource.ListRequest{
			ResourceType: "GCP::Compute::Subnetwork",
			TargetConfig: testutil.TargetConfig,
			PageSize:     2,
		}

		listResult, err := subnetwork.List(ctx, listReq)
		require.NoError(t, err, "List with pagination should not return error")
		require.NotNil(t, listResult, "List result should not be nil")

		t.Logf("First page: %d subnetworks", len(listResult.NativeIDs))

		if listResult.NextPageToken != nil && *listResult.NextPageToken != "" {
			t.Logf("Next page token present: %s", *listResult.NextPageToken)

			// Fetch next page
			listReq.PageToken = listResult.NextPageToken
			nextPageResult, err := subnetwork.List(ctx, listReq)
			require.NoError(t, err, "List next page should not return error")
			t.Logf("Second page: %d subnetworks", len(nextPageResult.NativeIDs))
		} else {
			t.Log("No next page token (less than 2 subnetworks or single page)")
		}
	})
}
