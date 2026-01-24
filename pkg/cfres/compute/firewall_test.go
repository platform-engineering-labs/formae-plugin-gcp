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

// TestFirewallCreate tests the creation, reading, and deletion of a GCP Firewall rule
func TestFirewallCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create provisioner instances using registry
	network, err := NewComputeProvisioner(testutil.Config, NetworkResourceType)
	require.NoError(t, err, "Failed to create network provisioner")
	firewall, err := NewComputeProvisioner(testutil.Config, FirewallResourceType)
	require.NoError(t, err, "Failed to create firewall provisioner")

	// Generate unique names
	networkName := fmt.Sprintf("formae-test-fw-net-%s", uuid.New().String()[:8])
	firewallName := fmt.Sprintf("formae-test-fw-%s", uuid.New().String()[:8])
	t.Logf("Creating test network: %s", networkName)
	t.Logf("Creating test firewall: %s", firewallName)

	ctx := context.Background()

	
	var networkSelfLink string
	var networkNativeID string

	// First, create a network for the firewall
	t.Run("SetupNetwork", func(t *testing.T) {
		networkProperties := map[string]interface{}{
			"name":                  networkName,
			"description":           "Test network for firewall integration test",
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

		networkProps, err := utils.ParseProperties(statusResult.ProgressResult.ResourceProperties)
		require.NoError(t, err, "Failed to parse network properties")

		networkSelfLink = utils.GetString(networkProps, "selfLink")
		require.NotEmpty(t, networkSelfLink, "Network selfLink should not be empty")

		networkNativeID = statusResult.ProgressResult.NativeID
		require.NotEmpty(t, networkNativeID, "Network native ID should not be empty")

		t.Logf("Network created: %s (selfLink: %s)", networkName, networkSelfLink)
	})

	// Test Firewall Create operation
	t.Run("CreateFirewall", func(t *testing.T) {
		firewallProperties := map[string]interface{}{
			"name":        firewallName,
			"description": "Test firewall rule created by Formae integration test",
			"network":     networkSelfLink,
			"direction":   "INGRESS",
			"priority":    1000,
			"sourceRanges": []string{
				"0.0.0.0/0",
			},
			"allowed": []map[string]interface{}{
				{
					"IPProtocol": "tcp",
					"ports":      []string{"80", "443"},
				},
				{
					"IPProtocol": "icmp",
				},
			},
			"targetTags": []string{"web", "http-server"},
			"logConfig": map[string]interface{}{
				"enable":   true,
				"metadata": "INCLUDE_ALL_METADATA",
			},
		}

		firewallPropsJSON, err := json.Marshal(firewallProperties)
		require.NoError(t, err, "Failed to marshal firewall properties")

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Compute::Firewall",
			Properties:   firewallPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		// Create the firewall
		createResult, err := firewall.Create(ctx, createReq)
		require.NoError(t, err, "Create operation should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation, "Operation should be Create")
		assert.Equal(t, resource.OperationStatusInProgress, createResult.ProgressResult.OperationStatus, "Should be in progress")
		require.NotEmpty(t, createResult.ProgressResult.RequestID, "RequestID should be set")
		require.NotEmpty(t, createResult.ProgressResult.NativeID, "NativeID should be set")

		t.Logf("Firewall creation initiated with RequestID: %s", createResult.ProgressResult.RequestID)

		// Wait for creation to complete
		statusResult, err := testutil.WaitForCreate(t, ctx, firewall, createResult, testutil.TargetConfig, "GCP::Compute::Firewall")
		require.NoError(t, err, "Firewall creation should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")

		nativeID := statusResult.ProgressResult.NativeID
		t.Logf("Firewall created with native ID: %s", nativeID)

		// Test Read operation
		t.Run("Read", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Compute::Firewall",
			}

			readResult, err := firewall.Read(ctx, readReq)
			require.NoError(t, err, "Read operation should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			require.Empty(t, readResult.ErrorCode, "Read should not have error code")
			require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

			// Verify properties
			readProps, err := utils.ParseProperties(readResult.Properties)
			require.NoError(t, err, "Failed to parse read properties")

			assert.Equal(t, firewallName, utils.GetString(readProps, "name"), "Firewall name should match")
			assert.Equal(t, "INGRESS", utils.GetString(readProps, "direction"), "Direction should match")
			assert.Equal(t, int32(1000), utils.GetInt32(readProps, "priority"), "Priority should match")

			// Verify allowed rules
			allowed := utils.GetArray(readProps, "allowed")
			require.NotEmpty(t, allowed, "Allowed rules should not be empty")
			assert.Len(t, allowed, 2, "Should have 2 allowed rules")

			// Verify target tags
			targetTags := utils.GetArray(readProps, "targetTags")
			require.NotEmpty(t, targetTags, "Target tags should not be empty")
			assert.Len(t, targetTags, 2, "Should have 2 target tags")

			// Verify log config
			logConfig := utils.GetObject(readProps, "logConfig")
			require.NotNil(t, logConfig, "Log config should exist")
			assert.True(t, utils.GetBool(logConfig, "enable"), "Logging should be enabled")

			t.Logf("Read firewall properties: %+v", readProps)
		})

		// Test List operation
		t.Run("List", func(t *testing.T) {
			listReq := &resource.ListRequest{
				ResourceType: "GCP::Compute::Firewall",
				TargetConfig: testutil.TargetConfig,
			}

			listResult, err := firewall.List(ctx, listReq)
			require.NoError(t, err, "List operation should not return error")
			require.NotNil(t, listResult, "List result should not be nil")
			require.NotNil(t, listResult.NativeIDs, "Resources list should not be nil")

			t.Logf("Found %d firewall rules in project %s", len(listResult.NativeIDs), testutil.Project)

			// Verify our firewall is in the list
			found := false
			for _, id := range listResult.NativeIDs {
				if id == nativeID {
					found = true
					break
				}
			}
			assert.True(t, found, "Created firewall should be in the list")
		})

		// Test Delete operation
		t.Run("Delete", func(t *testing.T) {
			deleteReq := &resource.DeleteRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Compute::Firewall",
			}

			deleteResult, err := firewall.Delete(ctx, deleteReq)
			require.NoError(t, err, "Delete operation should not return error")
			require.NotNil(t, deleteResult, "Delete result should not be nil")
			require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")

			assert.Equal(t, resource.OperationDelete, deleteResult.ProgressResult.Operation, "Operation should be Delete")
			assert.Equal(t, resource.OperationStatusInProgress, deleteResult.ProgressResult.OperationStatus, "Should be in progress")
			require.NotEmpty(t, deleteResult.ProgressResult.RequestID, "RequestID should be set")

			t.Logf("Firewall deletion initiated with RequestID: %s", deleteResult.ProgressResult.RequestID)

			// Wait for deletion to complete
			statusResult, err := testutil.WaitForDelete(t, ctx, firewall, deleteResult, testutil.TargetConfig, "GCP::Compute::Firewall")
			require.NoError(t, err, "Firewall deletion should complete successfully")
			require.NotNil(t, statusResult, "Status result should not be nil")

			t.Logf("Firewall deleted successfully")
		})

		// Verify deletion
		t.Run("VerifyDeleted", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Compute::Firewall",
			}

			readResult, err := firewall.Read(ctx, readReq)
			require.NoError(t, err, "Read should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode, "Should return NotFound")

			t.Logf("Verified firewall was deleted")
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

// TestFirewallEgress tests creating an egress firewall rule
func TestFirewallEgress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	network, err := NewComputeProvisioner(testutil.Config, NetworkResourceType)
	require.NoError(t, err, "Failed to create network provisioner")
	firewall, err := NewComputeProvisioner(testutil.Config, FirewallResourceType)
	require.NoError(t, err, "Failed to create firewall provisioner")

	networkName := fmt.Sprintf("formae-test-egress-net-%s", uuid.New().String()[:8])
	firewallName := fmt.Sprintf("formae-test-egress-fw-%s", uuid.New().String()[:8])

	t.Logf("Creating test for egress firewall rule")

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
		networkProps := utils.MustParseProperties(statusResult.ProgressResult.ResourceProperties)
		networkSelfLink = utils.GetString(networkProps, "selfLink")
		require.NotEmpty(t, networkSelfLink)
	})

	// Create egress firewall rule
	t.Run("CreateEgressFirewall", func(t *testing.T) {
		firewallProperties := map[string]interface{}{
			"name":        firewallName,
			"description": "Test egress firewall rule",
			"network":     networkSelfLink,
			"direction":   "EGRESS",
			"priority":    1000,
			"destinationRanges": []string{
				"10.0.0.0/8",
				"172.16.0.0/12",
			},
			"allowed": []map[string]interface{}{
				{
					"IPProtocol": "tcp",
					"ports":      []string{"443", "3306"},
				},
			},
			"targetTags": []string{"database-client"},
		}

		firewallPropsJSON, err := json.Marshal(firewallProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Compute::Firewall",
			Properties:   firewallPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := firewall.Create(ctx, createReq)
		require.NoError(t, err)

		statusResult, err := testutil.WaitForCreate(t, ctx, firewall, createResult, testutil.TargetConfig, "GCP::Compute::Firewall")
		require.NoError(t, err)

		nativeID := statusResult.ProgressResult.NativeID

		// Verify egress properties
		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Compute::Firewall",
		}

		readResult, err := firewall.Read(ctx, readReq)
		require.NoError(t, err)

		readProps := utils.MustParseProperties(readResult.Properties)
		assert.Equal(t, "EGRESS", utils.GetString(readProps, "direction"), "Direction should be EGRESS")

		destinationRanges := utils.GetArray(readProps, "destinationRanges")
		require.NotEmpty(t, destinationRanges, "Destination ranges should not be empty")
		assert.Len(t, destinationRanges, 2, "Should have 2 destination ranges")

		// Verify the actual CIDR ranges
		ranges := make([]string, len(destinationRanges))
		for i, r := range destinationRanges {
			ranges[i] = r.(string)
		}
		assert.Contains(t, ranges, "10.0.0.0/8", "Should contain 10.0.0.0/8")
		assert.Contains(t, ranges, "172.16.0.0/12", "Should contain 172.16.0.0/12")

		t.Logf("Egress firewall created successfully")

		// Cleanup firewall
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Compute::Firewall",
		}
		deleteResult, err := firewall.Delete(ctx, deleteReq)
		require.NoError(t, err)
		_, err = testutil.WaitForDelete(t, ctx, firewall, deleteResult, testutil.TargetConfig, "GCP::Compute::Firewall")
		require.NoError(t, err)

		t.Logf("Firewall deleted successfully")
	})

	// Cleanup network (must happen after firewall is deleted)
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

// TestFirewallDenyRules tests creating a firewall with deny rules
func TestFirewallDenyRules(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	network, err := NewComputeProvisioner(testutil.Config, NetworkResourceType)
	require.NoError(t, err, "Failed to create network provisioner")
	firewall, err := NewComputeProvisioner(testutil.Config, FirewallResourceType)
	require.NoError(t, err, "Failed to create firewall provisioner")

	networkName := fmt.Sprintf("formae-test-deny-net-%s", uuid.New().String()[:8])
	firewallName := fmt.Sprintf("formae-test-deny-fw-%s", uuid.New().String()[:8])

	ctx := context.Background()

	var networkSelfLink string
	var networkNativeID string

	// Create network
	t.Run("SetupNetwork", func(t *testing.T) {
		networkProperties := map[string]interface{}{
			"name":                  networkName,
			"autoCreateSubnetworks": false,
		}

		networkPropsJSON, err := json.Marshal(networkProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Compute::Network",
			Properties:   networkPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := network.Create(ctx, createReq)
		require.NoError(t, err)

		statusResult, err := testutil.WaitForCreate(t, ctx, network, createResult, testutil.TargetConfig, "GCP::Compute::Network")
		require.NoError(t, err)

		networkNativeID = statusResult.ProgressResult.NativeID
		networkProps := utils.MustParseProperties(statusResult.ProgressResult.ResourceProperties)
		networkSelfLink = utils.GetString(networkProps, "selfLink")
	})

	// Create deny firewall rule
	t.Run("CreateDenyFirewall", func(t *testing.T) {
		firewallProperties := map[string]interface{}{
			"name":     firewallName,
			"network":  networkSelfLink,
			"priority": 900,
			"sourceRanges": []string{
				"192.168.1.0/24",
			},
			"denied": []map[string]interface{}{
				{
					"IPProtocol": "tcp",
					"ports":      []string{"22", "3389"},
				},
			},
			"description": "Block SSH and RDP from specific subnet",
		}

		firewallPropsJSON, err := json.Marshal(firewallProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Compute::Firewall",
			Properties:   firewallPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := firewall.Create(ctx, createReq)
		require.NoError(t, err)

		statusResult, err := testutil.WaitForCreate(t, ctx, firewall, createResult, testutil.TargetConfig, "GCP::Compute::Firewall")
		require.NoError(t, err)

		nativeID := statusResult.ProgressResult.NativeID

		// Verify deny rules
		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Compute::Firewall",
		}

		readResult, err := firewall.Read(ctx, readReq)
		require.NoError(t, err)

		readProps := utils.MustParseProperties(readResult.Properties)
		denied := utils.GetArray(readProps, "denied")
		require.NotEmpty(t, denied, "Denied rules should not be empty")
		assert.Len(t, denied, 1, "Should have 1 deny rule")

		t.Logf("Deny firewall created successfully")

		// Cleanup
		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Compute::Firewall",
		}
		deleteResult, err := firewall.Delete(ctx, deleteReq)
		require.NoError(t, err)
		_, err = testutil.WaitForDelete(t, ctx, firewall, deleteResult, testutil.TargetConfig, "GCP::Compute::Firewall")
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

// TestFirewallUpdate tests updating a GCP Firewall
func TestFirewallUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	firewall, err := NewComputeProvisioner(testutil.Config, FirewallResourceType)
	require.NoError(t, err, "Failed to create firewall provisioner")
	firewallName := fmt.Sprintf("formae-test-fw-update-%s", uuid.New().String()[:8])
	networkName := fmt.Sprintf("formae-test-net-fw-update-%s", uuid.New().String()[:8])
	t.Logf("Creating test firewall for update: %s", firewallName)

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

	// Create firewall
	properties := map[string]interface{}{
		"name":         firewallName,
		"network":      fmt.Sprintf("projects/%s/global/networks/%s", testutil.Project, networkName),
		"direction":    "INGRESS",
		"priority":     1000,
		"sourceRanges": []string{"0.0.0.0/0"},
		"allowed": []map[string]interface{}{
			{
				"IPProtocol": "tcp",
				"ports":      []string{"80", "443"},
			},
		},
	}

	propertiesJSON, err := json.Marshal(properties)
	require.NoError(t, err)

	createReq := &resource.CreateRequest{
		ResourceType: "GCP::Compute::Firewall",
		Properties:   propertiesJSON,
		TargetConfig: testutil.TargetConfig,
	}

	createResult, err := firewall.Create(ctx, createReq)
	require.NoError(t, err)

	statusResult, err := testutil.WaitForCreate(t, ctx, firewall, createResult, testutil.TargetConfig, "GCP::Compute::Firewall")
	require.NoError(t, err)
	nativeID := statusResult.ProgressResult.NativeID

	// Update firewall
	t.Run("Update", func(t *testing.T) {
		updatedProperties := map[string]interface{}{
			"name":         firewallName,
			"network":      fmt.Sprintf("projects/%s/global/networks/%s", testutil.Project, networkName),
			"direction":    "INGRESS",
			"priority":     900,                    // Change priority
			"sourceRanges": []string{"10.0.0.0/8"}, // Change source ranges
			"allowed": []map[string]interface{}{
				{
					"IPProtocol": "tcp",
					"ports":      []string{"80", "443", "8080"}, // Add port 8080
				},
			},
			"description": "Updated firewall rule", // Add description
		}

		updatedPropsJSON, err := json.Marshal(updatedProperties)
		require.NoError(t, err)

		updateReq := &resource.UpdateRequest{
			ResourceType: "GCP::Compute::Firewall",
			DesiredProperties: updatedPropsJSON,
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
		}

		updateResult, err := firewall.Update(ctx, updateReq)
		require.NoError(t, err)
		require.NotNil(t, updateResult)
		assert.Equal(t, resource.OperationUpdate, updateResult.ProgressResult.Operation)
		assert.Equal(t, resource.OperationStatusInProgress, updateResult.ProgressResult.OperationStatus,
			"Update should return InProgress status, not Failed or Success")

		// Wait for update to complete
		if updateResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
			statusReq := &resource.StatusRequest{
				RequestID:    updateResult.ProgressResult.RequestID,
				ResourceType: "GCP::Compute::Firewall",
				TargetConfig: testutil.TargetConfig,
			}

			statusResult, err := testutil.WaitForStatus(t, ctx, firewall, statusReq)
			require.NoError(t, err)
			assert.Equal(t, resource.OperationStatusSuccess, statusResult.ProgressResult.OperationStatus)
		}

		// Verify update
		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			ResourceType: "GCP::Compute::Firewall",
			TargetConfig: testutil.TargetConfig,
		}

		readResult, err := firewall.Read(ctx, readReq)
		require.NoError(t, err)

		var readProps map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &readProps)
		require.NoError(t, err)

		assert.Equal(t, float64(900), readProps["priority"])
		assert.Equal(t, "Updated firewall rule", readProps["description"])
		t.Logf("Firewall updated successfully")
	})

	// Cleanup
	deleteReq := &resource.DeleteRequest{
		NativeID:     nativeID,
		ResourceType: "GCP::Compute::Firewall",
		TargetConfig: testutil.TargetConfig,
	}
	delResult, _ := firewall.Delete(ctx, deleteReq)
	testutil.WaitForDelete(t, ctx, firewall, delResult, testutil.TargetConfig, "GCP::Compute::Firewall")

	// Delete network
	netDeleteReq := &resource.DeleteRequest{
		NativeID: networkID,
		ResourceType: "GCP::Compute::Network",
		TargetConfig: testutil.TargetConfig,
	}
	netDelResult, _ := network.Delete(ctx, netDeleteReq)
	testutil.WaitForDelete(t, ctx, network, netDelResult, testutil.TargetConfig, "GCP::Compute::Network")
}
