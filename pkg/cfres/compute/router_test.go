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

	"github.com/google/uuid"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/testutil"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRouterCreate tests the creation, reading, and deletion of a GCP Router
func TestRouterCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create provisioner instances
	network, err := NewComputeProvisioner(testutil.Config, NetworkResourceType)
	require.NoError(t, err, "Failed to create network provisioner")
	router, err := NewComputeProvisioner(testutil.Config, RouterResourceType)
	require.NoError(t, err, "Failed to create router provisioner")

	// Generate unique names
	networkName := fmt.Sprintf("formae-test-net-%s", uuid.New().String()[:8])
	routerName := fmt.Sprintf("formae-test-router-%s", uuid.New().String()[:8])
	t.Logf("Creating test network: %s", networkName)
	t.Logf("Creating test router: %s", routerName)

	ctx := context.Background()

	var networkSelfLink string
	var networkNativeID string

	// First, create a network for the router
	t.Run("SetupNetwork", func(t *testing.T) {
		networkProperties := map[string]interface{}{
			"name":                  networkName,
			"description":           "Test network for router integration test",
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

		// Capture native ID for cleanup
		networkNativeID = statusResult.ProgressResult.NativeID
		require.NotEmpty(t, networkNativeID, "Network NativeID should not be empty")

		// Extract selfLink from network properties
		networkProps := utils.MustParseProperties(string(statusResult.ProgressResult.ResourceProperties))
		networkSelfLink = utils.GetString(networkProps, "selfLink")
		require.NotEmpty(t, networkSelfLink, "Network selfLink should not be empty")

		t.Logf("Network created: %s (nativeID: %s, selfLink: %s)", networkName, networkNativeID, networkSelfLink)
	})

	// Test Router Create operation
	t.Run("CreateRouter", func(t *testing.T) {
		routerProperties := map[string]interface{}{
			"name":        routerName,
			"description": "Test router created by Formae integration test",
			"network":     networkSelfLink,
			"bgp": map[string]interface{}{
				"asn":               64512,
				"advertiseMode":     "CUSTOM",
				"advertisedGroups":  []string{"ALL_SUBNETS"},
				"keepaliveInterval": 20,
			},
		}

		routerPropsJSON, err := json.Marshal(routerProperties)
		require.NoError(t, err, "Failed to marshal router properties")

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Compute::Router",
				Properties:   routerPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		// Create the router
		createResult, err := router.Create(ctx, createReq)
		require.NoError(t, err, "Create operation should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation, "Operation should be Create")
		if createResult.ProgressResult.OperationStatus == resource.OperationStatusFailure {
			t.Fatalf("Router creation failed: %s (error code: %s)",
				createResult.ProgressResult.StatusMessage,
				createResult.ProgressResult.ErrorCode)
		}
		assert.Equal(t, resource.OperationStatusInProgress, createResult.ProgressResult.OperationStatus, "Should be in progress")
		require.NotEmpty(t, createResult.ProgressResult.RequestID, "RequestID should be set")
		require.NotEmpty(t, createResult.ProgressResult.NativeID, "NativeID should be set")

		t.Logf("Router creation initiated with RequestID: %s", createResult.ProgressResult.RequestID)

		// Wait for creation to complete
		statusResult, err := testutil.WaitForCreate(t, ctx, router, createResult, testutil.TargetConfig, "GCP::Compute::Router")
		require.NoError(t, err, "Router creation should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")

		nativeID := statusResult.ProgressResult.NativeID
		t.Logf("Router created with native ID: %s", nativeID)

		// Test Read operation
		t.Run("Read", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Compute::Router",
			}

			readResult, err := router.Read(ctx, readReq)
			require.NoError(t, err, "Read operation should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			require.Empty(t, readResult.ErrorCode, "Read should not have error code")
			require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

			// Verify properties
			var readProps map[string]interface{}
			err = json.Unmarshal([]byte(readResult.Properties), &readProps)
			require.NoError(t, err, "Failed to unmarshal read properties")

			assert.Equal(t, routerName, readProps["name"], "Router name should match")
			assert.Equal(t, networkSelfLink, readProps["network"], "Network should match")

			// Verify BGP configuration
			if bgp, ok := readProps["bgp"].(map[string]interface{}); ok {
				assert.Equal(t, float64(64512), bgp["asn"], "ASN should match")
				assert.Equal(t, "CUSTOM", bgp["advertiseMode"], "Advertise mode should match")
				t.Logf("BGP configuration: %+v", bgp)
			}

			t.Logf("Read router properties: %+v", readProps)
		})

		// Test List operation
		t.Run("List", func(t *testing.T) {
			listReq := &resource.ListRequest{
				ResourceType: "GCP::Compute::Router",
				TargetConfig: testutil.TargetConfig,
			}

			listResult, err := router.List(ctx, listReq)
			require.NoError(t, err, "List operation should not return error")
			require.NotNil(t, listResult, "List result should not be nil")
			require.NotNil(t, listResult.NativeIDs, "Resources list should not be nil")

			t.Logf("Found %d routers in region %s", len(listResult.NativeIDs), testutil.Region)

			// Verify our router is in the list
			found := false
			for _, id := range listResult.NativeIDs {
				if id == nativeID {
					found = true
					break
				}
			}
			assert.True(t, found, "Created router should be in the list")
		})

		// Test Delete operation
		t.Run("Delete", func(t *testing.T) {
			deleteReq := &resource.DeleteRequest{
				NativeID: nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Compute::Router",
			}

			deleteResult, err := router.Delete(ctx, deleteReq)
			require.NoError(t, err, "Delete operation should not return error")
			require.NotNil(t, deleteResult, "Delete result should not be nil")
			require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")

			assert.Equal(t, resource.OperationDelete, deleteResult.ProgressResult.Operation, "Operation should be Delete")
			assert.Equal(t, resource.OperationStatusInProgress, deleteResult.ProgressResult.OperationStatus, "Should be in progress")
			require.NotEmpty(t, deleteResult.ProgressResult.RequestID, "RequestID should be set")

			t.Logf("Router deletion initiated with RequestID: %s", deleteResult.ProgressResult.RequestID)

			// Wait for deletion to complete
			statusResult, err := testutil.WaitForDelete(t, ctx, router, deleteResult, testutil.TargetConfig, "GCP::Compute::Router")
			require.NoError(t, err, "Router deletion should complete successfully")
			require.NotNil(t, statusResult, "Status result should not be nil")

			t.Logf("Router deleted successfully")
		})

		// Verify deletion
		t.Run("VerifyDeleted", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Compute::Router",
			}

			readResult, err := router.Read(ctx, readReq)
			require.NoError(t, err, "Read should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode, "Should return NotFound")

			t.Logf("Verified router was deleted")
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

		t.Logf("Network deleted: %s", networkName)
	})
}

// TestRouterWithBGPPeers tests creating a router with BGP peer configuration
func TestRouterWithBGPPeers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	network, err := NewComputeProvisioner(testutil.Config, NetworkResourceType)
	require.NoError(t, err, "Failed to create network provisioner")
	router, err := NewComputeProvisioner(testutil.Config, RouterResourceType)
	require.NoError(t, err, "Failed to create router provisioner")
	subnetwork, err := NewComputeProvisioner(testutil.Config, SubnetworkResourceType)
	require.NoError(t, err, "Failed to create subnetwork provisioner")

	networkName := fmt.Sprintf("formae-test-bgp-net-%s", uuid.New().String()[:8])
	routerName := fmt.Sprintf("formae-test-bgp-router-%s", uuid.New().String()[:8])
	subnetworkName := fmt.Sprintf("formae-test-bgp-subnet-%s", uuid.New().String()[:8])

	t.Logf("Creating network with router and BGP configuration")

	ctx := context.Background()

	var networkSelfLink string
	var networkNativeID string

	// Create network
	t.Run("SetupNetwork", func(t *testing.T) {
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

		statusResult, err := testutil.WaitForCreate(t, ctx, network, createResult, testutil.TargetConfig, "GCP::Compute::Network")
		require.NoError(t, err)

		networkNativeID = statusResult.ProgressResult.NativeID
		networkSelfLink = utils.GetString(utils.MustParseProperties(statusResult.ProgressResult.ResourceProperties), "selfLink")
		require.NotEmpty(t, networkSelfLink)
	})

	// Create subnetwork for router interface
	var subnetSelfLink string
	var subnetNativeID string
	t.Run("SetupSubnetwork", func(t *testing.T) {
		subnetProperties := map[string]interface{}{
			"name":        subnetworkName,
			"ipCidrRange": "10.0.1.0/24",
			"network":     networkSelfLink,
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

		subnetNativeID = statusResult.ProgressResult.NativeID
		subnetSelfLink = utils.GetString(utils.MustParseProperties(statusResult.ProgressResult.ResourceProperties), "selfLink")
		require.NotEmpty(t, subnetSelfLink)
	})

	// Create router with BGP peers
	var routerNativeID string
	t.Run("CreateRouterWithBGP", func(t *testing.T) {
		routerProperties := map[string]interface{}{
			"name":    routerName,
			"network": networkSelfLink,
			"bgp": map[string]interface{}{
				"asn":               64512,
				"advertiseMode":     "CUSTOM",
				"advertisedGroups":  []string{"ALL_SUBNETS"},
				"keepaliveInterval": 20,
				"advertisedIpRanges": []map[string]interface{}{
					{
						"range":       "192.168.1.0/24",
						"description": "Test advertised range",
					},
				},
			},
		}

		routerPropsJSON, err := json.Marshal(routerProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Compute::Router",
			Properties:   routerPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := router.Create(ctx, createReq)
		require.NoError(t, err)

		statusResult, err := testutil.WaitForCreate(t, ctx, router, createResult, testutil.TargetConfig, "GCP::Compute::Router")
		require.NoError(t, err)

		routerNativeID = statusResult.ProgressResult.NativeID

		// Verify router configuration
		readReq := &resource.ReadRequest{
			NativeID:     routerNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Compute::Router",
		}

		readResult, err := router.Read(ctx, readReq)
		require.NoError(t, err)

		// Verify BGP configuration
		bgp := utils.GetObject(utils.MustParseProperties(readResult.Properties), "bgp")

		require.NotNil(t, bgp, "BGP configuration should exist")
		assert.Equal(t, float64(64512), bgp["asn"], "ASN should match")

		t.Logf("Router with BGP created successfully")
	})

	// Cleanup router
	t.Run("CleanupRouter", func(t *testing.T) {
		if routerNativeID == "" {
			t.Skip("Router was not created, skipping cleanup")
		}
		deleteReq := &resource.DeleteRequest{
			NativeID:     routerNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Compute::Router",
		}
		deleteResult, err := router.Delete(ctx, deleteReq)
		require.NoError(t, err)
		_, err = testutil.WaitForDelete(t, ctx, router, deleteResult, testutil.TargetConfig, "GCP::Compute::Router")
		require.NoError(t, err)
	})

	// Cleanup subnetwork
	t.Run("CleanupSubnetwork", func(t *testing.T) {
		deleteReq := &resource.DeleteRequest{
			NativeID:     subnetNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Compute::Subnetwork",
		}
		deleteResult, err := subnetwork.Delete(ctx, deleteReq)
		require.NoError(t, err)
		_, err = testutil.WaitForDelete(t, ctx, subnetwork, deleteResult, testutil.TargetConfig, "GCP::Compute::Subnetwork")
		require.NoError(t, err)
	})

	// Cleanup network
	t.Run("CleanupNetwork", func(t *testing.T) {
		deleteReq := &resource.DeleteRequest{
			NativeID:     networkNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Compute::Network",
		}
		deleteResult, err := network.Delete(ctx, deleteReq)
		require.NoError(t, err)
		_, err = testutil.WaitForDelete(t, ctx, network, deleteResult, testutil.TargetConfig, "GCP::Compute::Network")
		require.NoError(t, err)
	})
}

// TestRouterUpdate tests updating a GCP Router
func TestRouterUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	router, err := NewComputeProvisioner(testutil.Config, RouterResourceType)
	require.NoError(t, err, "Failed to create router provisioner")
	routerName := fmt.Sprintf("formae-test-router-update-%s", uuid.New().String()[:8])
	networkName := fmt.Sprintf("formae-test-net-router-update-%s", uuid.New().String()[:8])
	t.Logf("Creating test router for update: %s", routerName)

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

	// Create router
	properties := map[string]interface{}{
		"name":        routerName,
		"network":     fmt.Sprintf("projects/%s/global/networks/%s", testutil.Project, networkName),
		"description": "Initial router",
		"bgp": map[string]interface{}{
			"asn":               64512,
			"advertiseMode":     "CUSTOM",
			"keepaliveInterval": 20,
		},
	}

	propertiesJSON, err := json.Marshal(properties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: "GCP::Compute::Router",
			Properties:   propertiesJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := router.Create(ctx, createReq)
	require.NoError(t, err)

	statusResult, err := testutil.WaitForCreate(t, ctx, router, createResult, testutil.TargetConfig, "GCP::Compute::Router")
	require.NoError(t, err)
	nativeID := statusResult.ProgressResult.NativeID

	// Update router
	t.Run("Update", func(t *testing.T) {
		updatedProperties := map[string]interface{}{
			"name":        routerName,
			"network":     fmt.Sprintf("projects/%s/global/networks/%s", testutil.Project, networkName),
			"description": "Updated router description", // Change description
			"bgp": map[string]interface{}{
				"asn":               64512,
				"advertiseMode":     "CUSTOM",
				"keepaliveInterval": 30, // Change keepalive interval
				"advertisedGroups":  []string{"ALL_SUBNETS"},
			},
		}

		updatedPropsJSON, err := json.Marshal(updatedProperties)
		require.NoError(t, err)

		updateReq := &resource.UpdateRequest{
			ResourceType: "GCP::Compute::Router",
				DesiredProperties: updatedPropsJSON,
			NativeID: nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		updateResult, err := router.Update(ctx, updateReq)
		require.NoError(t, err)
		require.NotNil(t, updateResult)
		assert.Equal(t, resource.OperationUpdate, updateResult.ProgressResult.Operation)
		assert.Equal(t, resource.OperationStatusInProgress, updateResult.ProgressResult.OperationStatus,
			"Update should return InProgress status, not Failed or Success")

		// Wait for update to complete
		if updateResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
			statusReq := &resource.StatusRequest{
				RequestID:    updateResult.ProgressResult.RequestID,
				ResourceType: "GCP::Compute::Router",
				TargetConfig: testutil.TargetConfig,
			}

			statusResult, err := testutil.WaitForStatus(t, ctx, router, statusReq)
			require.NoError(t, err)
			assert.Equal(t, resource.OperationStatusSuccess, statusResult.ProgressResult.OperationStatus)
		}

		// Verify update
		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			ResourceType: "GCP::Compute::Router",
			TargetConfig: testutil.TargetConfig,
		}

		readResult, err := router.Read(ctx, readReq)
		require.NoError(t, err)

		var readProps map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &readProps)
		require.NoError(t, err)

		assert.Equal(t, "Updated router description", readProps["description"])
		t.Logf("Router updated successfully")
	})

	// Cleanup
	deleteReq := &resource.DeleteRequest{
		NativeID: nativeID,
		ResourceType: "GCP::Compute::Router",
		TargetConfig: testutil.TargetConfig,
	}
	delResult, _ := router.Delete(ctx, deleteReq)
	testutil.WaitForDelete(t, ctx, router, delResult, testutil.TargetConfig, "GCP::Compute::Router")

	// Delete network
	netDeleteReq := &resource.DeleteRequest{
		NativeID: networkID,
		ResourceType: "GCP::Compute::Network",
		TargetConfig: testutil.TargetConfig,
	}
	netDelResult, _ := network.Delete(ctx, netDeleteReq)
	testutil.WaitForDelete(t, ctx, network, netDelResult, testutil.TargetConfig, "GCP::Compute::Network")
}

func TestRouterNotFound(t *testing.T) {
	router, err := NewComputeProvisioner(testutil.Config, RouterResourceType)
	require.NoError(t, err)

	// Use proper NativeID format: projects/{project}/regions/{region}/routers/{name}
	readReq := &resource.ReadRequest{
		NativeID:     fmt.Sprintf("projects/%s/regions/%s/routers/nonexistent-router", testutil.Project, testutil.Region),
		TargetConfig: testutil.TargetConfig,
		ResourceType: "GCP::Compute::Router",
	}

	readResult, err := router.Read(context.Background(), readReq)
	require.NoError(t, err, "Read should not return error")
	require.NotNil(t, readResult, "Read result should not be nil")
	assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode, "Should return NotFound")

	t.Logf("Verified router not found")
}
