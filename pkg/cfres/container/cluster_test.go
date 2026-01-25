// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration
// +build integration

package container

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/compute"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/testutil"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getPrivateClusterConfig() map[string]interface{} {
	return map[string]interface{}{
		"privateClusterConfig": map[string]interface{}{
			"enablePrivateNodes":    true,
			"enablePrivateEndpoint": false, // Keep control plane public for easier access
			"masterIpv4CidrBlock":   "172.16.0.0/28",
		},
		"masterAuthorizedNetworksConfig": map[string]interface{}{
			"enabled": true,
			"cidrBlocks": []map[string]interface{}{
				{
					"cidrBlock":   "0.0.0.0/0",
					"displayName": "All networks",
				},
			},
		},
	}
}

// setupNetworkForCluster creates a VPC network and subnetwork for GKE cluster testing.
// Set createNew=false and provide existing names to skip creation and reuse existing resources.
// Returns networkNativeID, subnetworkNativeID, networkSelfLink
func setupNetworkForCluster(
	t *testing.T,
	ctx context.Context,
	createNew bool,
	existingNetworkName string,
	existingSubnetworkName string,
) (networkNativeID, subnetworkNativeID, networkSelfLink string) {
	if !createNew {
		// Reuse existing resources for faster debugging
		t.Logf("Reusing existing network: %s and subnetwork: %s", existingNetworkName, existingSubnetworkName)

		// Read network to get selfLink and native ID
		network, _ := compute.NewComputeProvisioner(testutil.Config, compute.NetworkResourceType)
		subnetwork, _ := compute.NewComputeProvisioner(testutil.Config, compute.SubnetworkResourceType)

		listReq := &resource.ListRequest{
			ResourceType: "GCP::Compute::Network",
			TargetConfig: testutil.TargetConfig,
		}
		listResult, err := network.List(ctx, listReq)
		require.NoError(t, err, "Failed to list networks")

		for _, netID := range listResult.NativeIDs {
			readReq := &resource.ReadRequest{
				NativeID:     netID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Compute::Network",
			}
			readResult, err := network.Read(ctx, readReq)
			require.NoError(t, err, "Failed to read network")

			var netProps map[string]interface{}
			json.Unmarshal([]byte(readResult.Properties), &netProps)
			if utils.GetString(netProps, "name") == existingNetworkName {
				networkNativeID = netID
				networkSelfLink = utils.GetString(netProps, "selfLink")
				break
			}
		}

		require.NotEmpty(t, networkSelfLink, "Could not find existing network: %s", existingNetworkName)

		// Get subnetwork native ID
		listSubnetReq := &resource.ListRequest{
			ResourceType: "GCP::Compute::Subnetwork",
			TargetConfig: testutil.TargetConfig,
		}
		listSubnetResult, err := subnetwork.List(ctx, listSubnetReq)
		require.NoError(t, err, "Failed to list subnetworks")

		for _, subnetID := range listSubnetResult.NativeIDs {
			readReq := &resource.ReadRequest{
				NativeID:     subnetID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Compute::Subnetwork",
			}
			readResult, err := subnetwork.Read(ctx, readReq)
			require.NoError(t, err, "Failed to read subnetwork")

			var subnetProps map[string]interface{}
			json.Unmarshal([]byte(readResult.Properties), &subnetProps)
			if utils.GetString(subnetProps, "name") == existingSubnetworkName {
				subnetworkNativeID = subnetID
				break
			}
		}

		require.NotEmpty(t, subnetworkNativeID, "Could not find existing subnetwork: %s", existingSubnetworkName)

		return networkNativeID, subnetworkNativeID, networkSelfLink
	}

	// Create new resources
	network, _ := compute.NewComputeProvisioner(testutil.Config, compute.NetworkResourceType)
	subnetwork, _ := compute.NewComputeProvisioner(testutil.Config, compute.SubnetworkResourceType)

	networkName := fmt.Sprintf("formae-test-cluster-net-%s", uuid.New().String()[:8])
	subnetworkName := fmt.Sprintf("formae-test-cluster-subnet-%s", uuid.New().String()[:8])

	t.Logf("Creating test network: %s", networkName)
	t.Logf("Creating test subnetwork: %s", subnetworkName)

	// Create network
	t.Run("SetupNetwork", func(t *testing.T) {
		networkProperties := map[string]interface{}{
			"name":                  networkName,
			"description":           "Test network for GKE cluster integration test",
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
		var networkProps map[string]interface{}
		err = json.Unmarshal(statusResult.ProgressResult.ResourceProperties, &networkProps)
		require.NoError(t, err, "Failed to unmarshal network properties")
		networkSelfLink = utils.GetString(networkProps, "selfLink")
		require.NotEmpty(t, networkSelfLink, "Network selfLink should not be empty")

		t.Logf("Network created: %s (nativeID: %s, selfLink: %s)", networkName, networkNativeID, networkSelfLink)
	})

	// Create subnetwork with secondary ranges for GKE
	t.Run("SetupSubnetwork", func(t *testing.T) {
		subnetworkProperties := map[string]interface{}{
			"name":        subnetworkName,
			"description": "Test subnetwork for GKE cluster",
			"network":     networkSelfLink,
			"ipCidrRange": "10.0.0.0/24",
			"region":      testutil.Region,
			"secondaryIpRanges": []map[string]interface{}{
				{
					"rangeName":   "pods",
					"ipCidrRange": "10.1.0.0/16",
				},
				{
					"rangeName":   "services",
					"ipCidrRange": "10.2.0.0/16",
				},
			},
			"privateIpGoogleAccess": true,
		}

		subnetworkPropsJSON, err := json.Marshal(subnetworkProperties)
		require.NoError(t, err, "Failed to marshal subnetwork properties")

		createSubnetworkReq := &resource.CreateRequest{
			ResourceType: "GCP::Compute::Subnetwork",
			Properties:   subnetworkPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := subnetwork.Create(ctx, createSubnetworkReq)
		require.NoError(t, err, "Subnetwork create should not return error")

		statusResult, err := testutil.WaitForCreate(t, ctx, subnetwork, createResult, testutil.TargetConfig, "GCP::Compute::Subnetwork")
		require.NoError(t, err, "Subnetwork creation should complete successfully")

		subnetworkNativeID = statusResult.ProgressResult.NativeID
		var subnetworkProps map[string]interface{}
		err = json.Unmarshal(statusResult.ProgressResult.ResourceProperties, &subnetworkProps)
		require.NoError(t, err, "Failed to unmarshal subnetwork properties")

		t.Logf("Subnetwork created: %s (nativeID: %s)", subnetworkName, subnetworkNativeID)
	})

	return networkNativeID, subnetworkNativeID, networkSelfLink
}

// TestClusterCreate tests the creation, reading, and deletion of a GKE Cluster
func TestClusterCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	cluster, _ := NewContainerProvisioner(testutil.Config, ClusterResourceType)

	// Setup network infrastructure
	// For faster debugging, set createNew=false and provide existing resource names:
	//networkNativeID, subnetworkNativeID, _ := setupNetworkForCluster(t, ctx, false, "existing-network-name", "existing-subnet-name")
	networkNativeID, subnetworkNativeID, _ := setupNetworkForCluster(t, ctx, true, "", "")

	// Generate unique cluster name
	clusterName := fmt.Sprintf("formae-test-cluster-%s", uuid.New().String()[:8])
	t.Logf("Creating test cluster: %s", clusterName)

	// Test Cluster Create operation
	t.Run("CreateCluster", func(t *testing.T) {
		clusterProperties := map[string]interface{}{
			"name":             clusterName,
			"description":      "Test GKE cluster created by Formae integration test",
			"location":         testutil.Region,
			"network":          networkNativeID,
			"subnetwork":       subnetworkNativeID,
			"initialNodeCount": 1,
			"nodeConfig": map[string]interface{}{
				"machineType": "e2-medium",
				"diskSizeGb":  50,
				"diskType":    "pd-standard",
				"oauthScopes": []string{
					"https://www.googleapis.com/auth/cloud-platform",
				},
				"labels": map[string]string{
					"environment": "test",
					"managed-by":  "formae",
				},
				"tags": []string{"test-cluster"},
			},
			"ipAllocationPolicy": map[string]interface{}{
				"useIpAliases":               true,
				"clusterSecondaryRangeName":  "pods",
				"servicesSecondaryRangeName": "services",
			},
			"addonsConfig": map[string]interface{}{
				"httpLoadBalancing": map[string]interface{}{
					"disabled": false,
				},
				"horizontalPodAutoscaling": map[string]interface{}{
					"disabled": false,
				},
			},
			"networkConfig": map[string]interface{}{
				"enableIntraNodeVisibility": true,
			},
			"releaseChannel": map[string]interface{}{
				"channel": "REGULAR",
			},
			"resourceLabels": map[string]string{
				"team": "platform",
				"env":  "test",
			},
		}
		for k, v := range getPrivateClusterConfig() {
			clusterProperties[k] = v
		}
		clusterPropsJSON, err := json.Marshal(clusterProperties)
		require.NoError(t, err, "Failed to marshal cluster properties")

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Container::Cluster",
			Properties:   clusterPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		// Create the cluster
		createResult, err := cluster.Create(ctx, createReq)
		require.NoError(t, err, "Create operation should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation, "Operation should be Create")
		assert.Equal(t, resource.OperationStatusInProgress, createResult.ProgressResult.OperationStatus, "Should be in progress")
		require.NotEmpty(t, createResult.ProgressResult.RequestID, "RequestID should be set")
		require.NotEmpty(t, createResult.ProgressResult.NativeID, "NativeID should be set")

		t.Logf("Cluster creation initiated with RequestID: %s", createResult.ProgressResult.RequestID)

		// Wait for creation to complete (clusters take 5-10 minutes)
		pollConfig := testutil.DefaultPollConfig()
		pollConfig.MaxAttempts = 100
		pollConfig.CheckInterval = 10 * time.Second

		statusResult, err := testutil.WaitForCreateWithConfig(
			t,
			ctx,
			cluster,
			createResult,
			testutil.TargetConfig, "GCP::Container::Cluster", pollConfig)
		require.NoError(t, err, "Cluster creation should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")

		nativeID := statusResult.ProgressResult.NativeID
		t.Logf("Cluster created with native ID: %s", nativeID)

		// Test Read operation
		t.Run("Read", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Container::Cluster",
			}

			readResult, err := cluster.Read(ctx, readReq)
			require.NoError(t, err, "Read operation should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			require.Empty(t, readResult.ErrorCode, "Read should not have error code")
			require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

			// Verify properties
			var readProps map[string]interface{}
			err = json.Unmarshal([]byte(readResult.Properties), &readProps)
			require.NoError(t, err, "Failed to unmarshal read properties")

			assert.Equal(t, clusterName, utils.GetString(readProps, "name"), "Cluster name should match")
			assert.Equal(t, testutil.Region, utils.GetString(readProps, "location"), "Location should match")
			assert.Equal(t, "RUNNING", utils.GetString(readProps, "status"), "Cluster should be running")
			assert.NotEmpty(t, utils.GetString(readProps, "endpoint"), "Endpoint should be set")
			assert.NotEmpty(t, utils.GetString(readProps, "currentMasterVersion"), "Master version should be set")

			// Verify node config
			nodeConfig := utils.GetObject(readProps, "nodeConfig")
			require.NotNil(t, nodeConfig, "Node config should exist")
			assert.Equal(t, "e2-medium", utils.GetString(nodeConfig, "machineType"), "Machine type should match")

			// Verify IP allocation policy
			ipAllocationPolicy := utils.GetObject(readProps, "ipAllocationPolicy")
			require.NotNil(t, ipAllocationPolicy, "IP allocation policy should exist")
			assert.True(t, utils.GetBool(ipAllocationPolicy, "useIpAliases"), "IP aliases should be enabled")
			assert.Equal(t, "pods", utils.GetString(ipAllocationPolicy, "clusterSecondaryRangeName"), "Pod range name should match")
			assert.Equal(t, "services", utils.GetString(ipAllocationPolicy, "servicesSecondaryRangeName"), "Services range name should match")

			// Verify resource labels
			resourceLabels := utils.GetObject(readProps, "resourceLabels")
			require.NotNil(t, resourceLabels, "Resource labels should exist")
			assert.Equal(t, "platform", utils.GetString(resourceLabels, "team"), "Team label should match")
			assert.Equal(t, "test", utils.GetString(resourceLabels, "env"), "Env label should match")

			// Verify addons config
			addonsConfig := utils.GetObject(readProps, "addonsConfig")
			require.NotNil(t, addonsConfig, "Addons config should exist")

			// Verify network config
			networkConfig := utils.GetObject(readProps, "networkConfig")
			require.NotNil(t, networkConfig, "Network config should exist")
			assert.True(t, utils.GetBool(networkConfig, "enableIntraNodeVisibility"), "Intra-node visibility should be enabled")

			// Verify release channel
			releaseChannel := utils.GetObject(readProps, "releaseChannel")
			require.NotNil(t, releaseChannel, "Release channel should exist")

			t.Logf("Read cluster properties successfully")
		})

		// Test List operation
		t.Run("List", func(t *testing.T) {
			listReq := &resource.ListRequest{
				ResourceType: "GCP::Container::Cluster",
				TargetConfig: testutil.TargetConfig,
			}

			listResult, err := cluster.List(ctx, listReq)
			require.NoError(t, err, "List operation should not return error")
			require.NotNil(t, listResult, "List result should not be nil")
			require.NotNil(t, listResult.NativeIDs, "Resources list should not be nil")

			t.Logf("Found %d clusters in region %s", len(listResult.NativeIDs), testutil.Region)

			// Verify our cluster is in the list
			found := false
			for _, id := range listResult.NativeIDs {
				if id == nativeID {
					found = true
					break
				}
			}
			assert.True(t, found, "Created cluster should be in the list")
		})

		// Test Delete operation
		t.Run("Delete", func(t *testing.T) {
			deleteReq := &resource.DeleteRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Container::Cluster",
			}

			deleteResult, err := cluster.Delete(ctx, deleteReq)
			require.NoError(t, err, "Delete operation should not return error")
			require.NotNil(t, deleteResult, "Delete result should not be nil")
			require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")

			assert.Equal(t, resource.OperationDelete, deleteResult.ProgressResult.Operation, "Operation should be Delete")
			assert.Equal(t, resource.OperationStatusInProgress, deleteResult.ProgressResult.OperationStatus, "Should be in progress")
			require.NotEmpty(t, deleteResult.ProgressResult.RequestID, "RequestID should be set")

			t.Logf("Cluster deletion initiated with RequestID: %s", deleteResult.ProgressResult.RequestID)

			// Wait for deletion to complete (clusters take 5-10 minutes to delete)
			statusResult, err := testutil.WaitForDeleteWithConfig(t, ctx, cluster, deleteResult, testutil.TargetConfig, "GCP::Container::Cluster", pollConfig)
			require.NoError(t, err, "Cluster deletion should complete successfully")
			require.NotNil(t, statusResult, "Status result should not be nil")

			t.Logf("Cluster deleted successfully")
		})

		// Verify deletion
		t.Run("VerifyDeleted", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Container::Cluster",
			}

			readResult, err := cluster.Read(ctx, readReq)
			require.NoError(t, err, "Read should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode, "Should return NotFound")

			t.Logf("Verified cluster was deleted")
		})
	})

	// Cleanup: Delete the subnetwork
	t.Run("CleanupSubnetwork", func(t *testing.T) {
		subnetwork, _ := compute.NewComputeProvisioner(testutil.Config, compute.SubnetworkResourceType)

		deleteReq := &resource.DeleteRequest{
			NativeID:     subnetworkNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Compute::Subnetwork",
		}

		deleteResult, err := subnetwork.Delete(ctx, deleteReq)
		require.NoError(t, err, "Subnetwork delete should not return error")

		_, err = testutil.WaitForDelete(t, ctx, subnetwork, deleteResult, testutil.TargetConfig, "GCP::Compute::Subnetwork")
		require.NoError(t, err, "Subnetwork deletion should complete successfully")

		t.Logf("Subnetwork deleted: %s", subnetworkNativeID)
	})

	// Cleanup: Delete the network
	t.Run("CleanupNetwork", func(t *testing.T) {
		network, _ := compute.NewComputeProvisioner(testutil.Config, compute.NetworkResourceType)

		deleteReq := &resource.DeleteRequest{
			NativeID:     networkNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Compute::Network",
		}

		deleteResult, err := network.Delete(ctx, deleteReq)
		require.NoError(t, err, "Network delete should not return error")

		_, err = testutil.WaitForDelete(t, ctx, network, deleteResult, testutil.TargetConfig, "GCP::Compute::Network")
		require.NoError(t, err, "Network deletion should complete successfully")

		t.Logf("Network deleted: %s", networkNativeID)
	})
}

// TestClusterWithPrivateNodes tests creating a private GKE cluster
func TestClusterWithPrivateNodes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	network, _ := compute.NewComputeProvisioner(testutil.Config, compute.NetworkResourceType)
	subnetwork, _ := compute.NewComputeProvisioner(testutil.Config, compute.SubnetworkResourceType)
	cluster, _ := NewContainerProvisioner(testutil.Config, ClusterResourceType)

	networkName := fmt.Sprintf("formae-test-private-net-%s", uuid.New().String()[:8])
	subnetworkName := fmt.Sprintf("formae-test-private-subnet-%s", uuid.New().String()[:8])
	clusterName := fmt.Sprintf("formae-test-private-%s", uuid.New().String()[:8])

	t.Logf("Creating test for private GKE cluster")

	ctx := context.Background()
	networkNativeID := ""
	subnetworkNativeID := ""
	clusterNativeID := ""
	networkSelfLink := ""

	// Cleanup resources if test fails
	defer func() {
		// Delete cluster first (if exists)
		if clusterNativeID != "" {
			t.Logf("Cleaning up cluster: %s", clusterNativeID)
			deleteReq := &resource.DeleteRequest{
				NativeID:     clusterNativeID,
				ResourceType: "GCP::Container::Cluster",
				TargetConfig: testutil.TargetConfig,
			}
			deleteRes, err := cluster.Delete(ctx, deleteReq)
			if err != nil {
				t.Logf("Failed to delete cluster: %v", err)
			} else if deleteRes != nil && deleteRes.ProgressResult != nil {
				if deleteRes.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
					_, _ = testutil.WaitForDelete(t, ctx, cluster, deleteRes, testutil.TargetConfig, "GCP::Container::Cluster")
				}
				t.Logf("Cluster deleted: %s", clusterNativeID)
			}
		}

		// Delete subnetwork
		if subnetworkNativeID != "" {
			t.Logf("Cleaning up subnetwork: %s", subnetworkNativeID)
			deleteReq := &resource.DeleteRequest{
				NativeID:     subnetworkNativeID,
				ResourceType: "GCP::Compute::Subnetwork",
				TargetConfig: testutil.TargetConfig,
			}
			deleteRes, err := subnetwork.Delete(ctx, deleteReq)
			if err != nil {
				t.Logf("Failed to delete subnetwork: %v", err)
			} else if deleteRes != nil && deleteRes.ProgressResult != nil {
				if deleteRes.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
					_, _ = testutil.WaitForDelete(t, ctx, subnetwork, deleteRes, testutil.TargetConfig, "GCP::Compute::Subnetwork")
				}
				t.Logf("Subnetwork deleted: %s", subnetworkNativeID)
			}
		}

		// Delete network
		if networkNativeID != "" {
			t.Logf("Cleaning up network: %s", networkNativeID)
			deleteReq := &resource.DeleteRequest{
				NativeID:     networkNativeID,
				ResourceType: "GCP::Compute::Network",
				TargetConfig: testutil.TargetConfig,
			}
			deleteRes, err := network.Delete(ctx, deleteReq)
			if err != nil {
				t.Logf("Failed to delete network: %v", err)
			} else if deleteRes != nil && deleteRes.ProgressResult != nil {
				if deleteRes.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
					_, _ = testutil.WaitForDelete(t, ctx, network, deleteRes, testutil.TargetConfig, "GCP::Compute::Network")
				}
				t.Logf("Network deleted: %s", networkNativeID)
			}
		}
	}()

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
		var networkProps map[string]interface{}
		err = json.Unmarshal(statusResult.ProgressResult.ResourceProperties, &networkProps)
		require.NoError(t, err)
		networkSelfLink = utils.GetString(networkProps, "selfLink")
	})

	// Create subnetwork
	t.Run("SetupSubnetwork", func(t *testing.T) {
		subnetworkProperties := map[string]interface{}{
			"name":        subnetworkName,
			"network":     networkSelfLink,
			"ipCidrRange": "10.10.0.0/24",
			"region":      testutil.Region,
			"secondaryIpRanges": []map[string]interface{}{
				{
					"rangeName":   "pods",
					"ipCidrRange": "10.11.0.0/16",
				},
				{
					"rangeName":   "services",
					"ipCidrRange": "10.12.0.0/16",
				},
			},
			"privateIpGoogleAccess": true,
		}

		subnetworkPropsJSON, err := json.Marshal(subnetworkProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Compute::Subnetwork",
			Properties:   subnetworkPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := subnetwork.Create(ctx, createReq)
		require.NoError(t, err)

		statusResult, err := testutil.WaitForCreate(t, ctx, subnetwork, createResult, testutil.TargetConfig, "GCP::Compute::Subnetwork")
		require.NoError(t, err)
		subnetworkNativeID = statusResult.ProgressResult.NativeID
	})

	// Create private cluster
	t.Run("CreatePrivateCluster", func(t *testing.T) {
		clusterProperties := map[string]interface{}{
			"name":             clusterName,
			"location":         testutil.Region,
			"network":          networkName,
			"subnetwork":       subnetworkName,
			"initialNodeCount": 1,
			"nodeConfig": map[string]interface{}{
				"machineType": "e2-medium",
				"oauthScopes": []string{
					"https://www.googleapis.com/auth/cloud-platform",
				},
			},
			"ipAllocationPolicy": map[string]interface{}{
				"useIpAliases":               true,
				"clusterSecondaryRangeName":  "pods",
				"servicesSecondaryRangeName": "services",
			},
			"privateClusterConfig": map[string]interface{}{
				"enablePrivateNodes":    true,
				"enablePrivateEndpoint": false,
				"masterIpv4CidrBlock":   "172.16.0.0/28",
			},
			"masterAuthorizedNetworksConfig": map[string]interface{}{
				"enabled": true,
				"cidrBlocks": []map[string]interface{}{
					{
						"cidrBlock":   "0.0.0.0/0",
						"displayName": "All networks",
					},
				},
			},
		}

		clusterPropsJSON, err := json.Marshal(clusterProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Container::Cluster",
			Properties:   clusterPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := cluster.Create(ctx, createReq)
		require.NoError(t, err)

		// GKE private cluster creation can take 15+ minutes
		pollConfig := testutil.NewPollConfig().ForCreate().WithResourceType("GCP::Container::Cluster").Build()
		pollConfig.MaxAttempts = 150
		pollConfig.CheckInterval = 10 * time.Second

		statusResult, err := testutil.WaitForCreateWithConfig(t, ctx, cluster, createResult, testutil.TargetConfig, "GCP::Container::Cluster", pollConfig)
		require.NoError(t, err)

		clusterNativeID = statusResult.ProgressResult.NativeID

		// Verify private cluster properties
		readReq := &resource.ReadRequest{
			NativeID:     clusterNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Container::Cluster",
		}

		readResult, err := cluster.Read(ctx, readReq)
		require.NoError(t, err)

		var readProps map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &readProps)
		require.NoError(t, err)

		privateClusterConfig := utils.GetObject(readProps, "privateClusterConfig")
		require.NotNil(t, privateClusterConfig, "Private cluster config should exist")
		assert.True(t, utils.GetBool(privateClusterConfig, "enablePrivateNodes"), "Private nodes should be enabled")
		assert.NotEmpty(t, utils.GetString(privateClusterConfig, "privateEndpoint"), "Private endpoint should be set")

		t.Logf("Private cluster created successfully")

		// Cleanup cluster
		deleteReq := &resource.DeleteRequest{
			NativeID:     clusterNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Container::Cluster",
		}
		deleteResult, err := cluster.Delete(ctx, deleteReq)
		require.NoError(t, err)
		_, err = testutil.WaitForDelete(t, ctx, cluster, deleteResult, testutil.TargetConfig, "GCP::Container::Cluster")
		require.NoError(t, err)

		// Clear so defer doesn't double-delete
		clusterNativeID = ""
	})

	// Cleanup subnetwork
	t.Run("CleanupSubnetwork", func(t *testing.T) {
		deleteReq := &resource.DeleteRequest{
			NativeID:     subnetworkNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Compute::Subnetwork",
		}
		deleteResult, err := subnetwork.Delete(ctx, deleteReq)
		require.NoError(t, err)
		_, err = testutil.WaitForDelete(t, ctx, subnetwork, deleteResult, testutil.TargetConfig, "GCP::Compute::Subnetwork")
		require.NoError(t, err)

		// Clear so defer doesn't double-delete
		subnetworkNativeID = ""
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

		// Clear so defer doesn't double-delete
		networkNativeID = ""
	})
}

func TestClusterReadSpecific(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Use a cluster name that doesn't exist
	specificClusterName := "non-existent-cluster-" + uuid.New().String()[:8]
	specificLocation := testutil.Region

	cluster, _ := NewContainerProvisioner(testutil.Config, ClusterResourceType)
	ctx := context.Background()

	nativeID := BuildClusterPath(testutil.Project, specificLocation, specificClusterName)
	t.Logf("Attempting to read non-existent cluster with native ID: %s", nativeID)

	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
		ResourceType: "GCP::Container::Cluster",
	}

	readResult, err := cluster.Read(ctx, readReq)
	require.NoError(t, err, "Read operation should not return error")
	require.NotNil(t, readResult, "Read result should not be nil")

	// Verify that we get a NotFound error code
	assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode,
		"Reading non-existent cluster should return NotFound error code")
	assert.Empty(t, readResult.Properties,
		"Properties should be empty for non-existent cluster")

	t.Logf("Successfully verified that non-existent cluster returns NotFound error code")
}

// TestClusterWithAutopilot tests creating an Autopilot GKE cluster
func TestClusterWithAutopilot(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	network, _ := compute.NewComputeProvisioner(testutil.Config, compute.NetworkResourceType)
	subnetwork, _ := compute.NewComputeProvisioner(testutil.Config, compute.SubnetworkResourceType)
	cluster, _ := NewContainerProvisioner(testutil.Config, ClusterResourceType)

	networkName := fmt.Sprintf("formae-test-autopilot-net-%s", uuid.New().String()[:8])
	subnetworkName := fmt.Sprintf("formae-test-autopilot-subnet-%s", uuid.New().String()[:8])
	clusterName := fmt.Sprintf("formae-test-autopilot-%s", uuid.New().String()[:8])

	ctx := context.Background()
	networkNativeID := ""
	subnetworkNativeID := ""
	clusterNativeID := ""
	networkSelfLink := ""

	// Cleanup resources if test fails
	defer func() {
		// Delete cluster first (if exists)
		if clusterNativeID != "" {
			t.Logf("Cleaning up cluster: %s", clusterNativeID)
			deleteReq := &resource.DeleteRequest{
				NativeID:     clusterNativeID,
				ResourceType: "GCP::Container::Cluster",
				TargetConfig: testutil.TargetConfig,
			}
			deleteRes, err := cluster.Delete(ctx, deleteReq)
			if err != nil {
				t.Logf("Failed to delete cluster: %v", err)
			} else if deleteRes != nil && deleteRes.ProgressResult != nil {
				if deleteRes.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
					_, _ = testutil.WaitForDelete(t, ctx, cluster, deleteRes, testutil.TargetConfig, "GCP::Container::Cluster")
				}
				t.Logf("Cluster deleted: %s", clusterNativeID)
			}
		}

		// Delete subnetwork
		if subnetworkNativeID != "" {
			t.Logf("Cleaning up subnetwork: %s", subnetworkNativeID)
			deleteReq := &resource.DeleteRequest{
				NativeID:     subnetworkNativeID,
				ResourceType: "GCP::Compute::Subnetwork",
				TargetConfig: testutil.TargetConfig,
			}
			deleteRes, err := subnetwork.Delete(ctx, deleteReq)
			if err != nil {
				t.Logf("Failed to delete subnetwork: %v", err)
			} else if deleteRes != nil && deleteRes.ProgressResult != nil {
				if deleteRes.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
					_, _ = testutil.WaitForDelete(t, ctx, subnetwork, deleteRes, testutil.TargetConfig, "GCP::Compute::Subnetwork")
				}
				t.Logf("Subnetwork deleted: %s", subnetworkNativeID)
			}
		}

		// Delete network
		if networkNativeID != "" {
			t.Logf("Cleaning up network: %s", networkNativeID)
			deleteReq := &resource.DeleteRequest{
				NativeID:     networkNativeID,
				ResourceType: "GCP::Compute::Network",
				TargetConfig: testutil.TargetConfig,
			}
			deleteRes, err := network.Delete(ctx, deleteReq)
			if err != nil {
				t.Logf("Failed to delete network: %v", err)
			} else if deleteRes != nil && deleteRes.ProgressResult != nil {
				if deleteRes.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
					_, _ = testutil.WaitForDelete(t, ctx, network, deleteRes, testutil.TargetConfig, "GCP::Compute::Network")
				}
				t.Logf("Network deleted: %s", networkNativeID)
			}
		}
	}()

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
		var networkProps map[string]interface{}
		err = json.Unmarshal(statusResult.ProgressResult.ResourceProperties, &networkProps)
		require.NoError(t, err)
		networkSelfLink = utils.GetString(networkProps, "selfLink")
	})

	// Create subnetwork
	t.Run("SetupSubnetwork", func(t *testing.T) {
		subnetworkProperties := map[string]interface{}{
			"name":        subnetworkName,
			"network":     networkSelfLink,
			"ipCidrRange": "10.20.0.0/24",
			"region":      testutil.Region,
			"secondaryIpRanges": []map[string]interface{}{
				{
					"rangeName":   "pods",
					"ipCidrRange": "10.21.0.0/16",
				},
				{
					"rangeName":   "services",
					"ipCidrRange": "10.22.0.0/16",
				},
			},
		}

		subnetworkPropsJSON, err := json.Marshal(subnetworkProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Compute::Subnetwork",
			Properties:   subnetworkPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := subnetwork.Create(ctx, createReq)
		require.NoError(t, err)

		statusResult, err := testutil.WaitForCreate(t, ctx, subnetwork, createResult, testutil.TargetConfig, "GCP::Compute::Subnetwork")
		require.NoError(t, err)
		subnetworkNativeID = statusResult.ProgressResult.NativeID
	})

	// Create autopilot cluster
	t.Run("CreateAutopilotCluster", func(t *testing.T) {
		clusterProperties := map[string]interface{}{
			"name":       clusterName,
			"location":   testutil.Region,
			"network":    networkName,
			"subnetwork": subnetworkName,
			"ipAllocationPolicy": map[string]interface{}{
				"useIpAliases":               true,
				"clusterSecondaryRangeName":  "pods",
				"servicesSecondaryRangeName": "services",
			},
			"autopilot": map[string]interface{}{
				"enabled": true,
			},
			"releaseChannel": map[string]interface{}{
				"channel": "REGULAR",
			},
		}

		clusterPropsJSON, err := json.Marshal(clusterProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Container::Cluster",
			Properties:   clusterPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := cluster.Create(ctx, createReq)
		require.NoError(t, err)

		statusResult, err := testutil.WaitForCreate(t, ctx, cluster, createResult, testutil.TargetConfig, "GCP::Container::Cluster")
		require.NoError(t, err)

		clusterNativeID = statusResult.ProgressResult.NativeID

		// Verify autopilot configuration
		readReq := &resource.ReadRequest{
			NativeID:     clusterNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Container::Cluster",
		}

		readResult, err := cluster.Read(ctx, readReq)
		require.NoError(t, err)

		var readProps map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &readProps)
		require.NoError(t, err)

		autopilot := utils.GetObject(readProps, "autopilot")
		require.NotNil(t, autopilot, "Autopilot config should exist")
		assert.True(t, utils.GetBool(autopilot, "enabled"), "Autopilot should be enabled")

		t.Logf("Autopilot cluster created successfully")

		// Cleanup cluster
		deleteReq := &resource.DeleteRequest{
			NativeID:     clusterNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Container::Cluster",
		}
		deleteResult, err := cluster.Delete(ctx, deleteReq)
		require.NoError(t, err)
		_, err = testutil.WaitForDelete(t, ctx, cluster, deleteResult, testutil.TargetConfig, "GCP::Container::Cluster")
		require.NoError(t, err)

		// Clear so defer doesn't double-delete
		clusterNativeID = ""
	})

	// Cleanup subnetwork
	t.Run("CleanupSubnetwork", func(t *testing.T) {
		deleteReq := &resource.DeleteRequest{
			NativeID:     subnetworkNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Compute::Subnetwork",
		}
		deleteResult, err := subnetwork.Delete(ctx, deleteReq)
		require.NoError(t, err)
		_, err = testutil.WaitForDelete(t, ctx, subnetwork, deleteResult, testutil.TargetConfig, "GCP::Compute::Subnetwork")
		require.NoError(t, err)

		// Clear so defer doesn't double-delete
		subnetworkNativeID = ""
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

		// Clear so defer doesn't double-delete
		networkNativeID = ""
	})
}
