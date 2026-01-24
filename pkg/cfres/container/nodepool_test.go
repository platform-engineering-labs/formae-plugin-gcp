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

// setupClusterForNodePool creates a cluster (with network/subnetwork) for node pool testing.
// Set createNew=false and provide existing names to skip creation and reuse existing resources.
// Returns networkNativeID, subnetworkNativeID, clusterName, clusterNativeID
func setupClusterForNodePool(
	t *testing.T,
	ctx context.Context,
	createNew bool,
	existingNetworkName string,
	existingSubnetworkName string,
	existingClusterName string,
) (networkNativeID, subnetworkNativeID, clusterName, clusterNativeID string) {
	pollConfig := testutil.DefaultPollConfig()
	pollConfig.MaxAttempts = 100
	pollConfig.CheckInterval = 10 * time.Second

	if !createNew {
		// Reuse existing resources for faster debugging
		t.Logf("Reusing existing network: %s, subnetwork: %s, cluster: %s",
			existingNetworkName, existingSubnetworkName, existingClusterName)

		// Read cluster to get native ID
		cluster, _ := NewContainerProvisioner(testutil.Config, ClusterResourceType)

		// Build cluster native ID from name
		clusterNativeID = BuildClusterPath(testutil.Project, testutil.Region, existingClusterName)

		// Verify cluster exists by reading it
		readReq := &resource.ReadRequest{
			NativeID:     clusterNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Container::Cluster",
		}
		readResult, err := cluster.Read(ctx, readReq)
		require.NoError(t, err, "Failed to read existing cluster")
		require.Empty(t, readResult.ErrorCode, "Cluster should exist")

		return existingNetworkName, existingSubnetworkName, existingClusterName, clusterNativeID
	}

	// Create new resources
	network, _ := compute.NewComputeProvisioner(testutil.Config, compute.NetworkResourceType)
	subnetwork, _ := compute.NewComputeProvisioner(testutil.Config, compute.SubnetworkResourceType)
	cluster, _ := NewContainerProvisioner(testutil.Config, ClusterResourceType)

	// Local variables for resource names (used during creation)
	networkName := fmt.Sprintf("formae-test-np-net-%s", uuid.New().String()[:8])
	subnetworkName := fmt.Sprintf("formae-test-np-subnet-%s", uuid.New().String()[:8])
	clusterName = fmt.Sprintf("formae-test-np-cluster-%s", uuid.New().String()[:8])

	t.Logf("Creating test network: %s", networkName)
	t.Logf("Creating test subnetwork: %s", subnetworkName)
	t.Logf("Creating test cluster: %s", clusterName)

	networkSelfLink := ""

	// Setup network
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
		var networkProps map[string]interface{}
		err = json.Unmarshal(statusResult.ProgressResult.ResourceProperties, &networkProps)
		require.NoError(t, err)
		networkSelfLink = utils.GetString(networkProps, "selfLink")
		require.NotEmpty(t, networkSelfLink)

		t.Logf("Network created: %s (nativeID: %s)", networkName, networkNativeID)
	})

	// Setup subnetwork
	t.Run("SetupSubnetwork", func(t *testing.T) {
		subnetworkProperties := map[string]interface{}{
			"name":        subnetworkName,
			"network":     networkSelfLink,
			"ipCidrRange": "10.30.0.0/24",
			"region":      testutil.Region,
			"secondaryIpRanges": []map[string]interface{}{
				{
					"rangeName":   "pods",
					"ipCidrRange": "10.31.0.0/16",
				},
				{
					"rangeName":   "services",
					"ipCidrRange": "10.32.0.0/16",
				},
			},
			"privateIpGoogleAccess": true,
		}

		subnetworkPropsJSON, err := json.Marshal(subnetworkProperties)
		require.NoError(t, err)

		createSubnetworkReq := &resource.CreateRequest{
			ResourceType: "GCP::Compute::Subnetwork",
			Properties:   subnetworkPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := subnetwork.Create(ctx, createSubnetworkReq)
		require.NoError(t, err)

		statusResult, err := testutil.WaitForCreate(t, ctx, subnetwork, createResult, testutil.TargetConfig, "GCP::Compute::Subnetwork")
		require.NoError(t, err)
		subnetworkNativeID = statusResult.ProgressResult.NativeID

		t.Logf("Subnetwork created: %s (nativeID: %s)", subnetworkName, subnetworkNativeID)
	})

	// Setup cluster
	t.Run("SetupCluster", func(t *testing.T) {
		clusterProperties := map[string]interface{}{
			"name":             clusterName,
			"location":         testutil.Region,
			"network":          networkName,
			"subnetwork":       subnetworkName,
			"initialNodeCount": 1,
			"nodeConfig": map[string]interface{}{
				"machineType": "e2-medium",
				"diskSizeGb":  50,
				"oauthScopes": []string{
					"https://www.googleapis.com/auth/cloud-platform",
				},
			},
			"ipAllocationPolicy": map[string]interface{}{
				"useIpAliases":               true,
				"clusterSecondaryRangeName":  "pods",
				"servicesSecondaryRangeName": "services",
			},
			"releaseChannel": map[string]interface{}{
				"channel": "REGULAR",
			},
		}

		// Add private cluster config
		for k, v := range getPrivateClusterConfig() {
			clusterProperties[k] = v
		}

		clusterPropsJSON, err := json.Marshal(clusterProperties)
		require.NoError(t, err)

		createClusterReq := &resource.CreateRequest{
			ResourceType: "GCP::Container::Cluster",
			Properties:   clusterPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := cluster.Create(ctx, createClusterReq)
		require.NoError(t, err)

		statusResult, err := testutil.WaitForCreateWithConfig(t, ctx, cluster, createResult, testutil.TargetConfig, "GCP::Container::Cluster", pollConfig)
		require.NoError(t, err)

		clusterNativeID = statusResult.ProgressResult.NativeID
		t.Logf("Cluster created: %s", clusterName)
	})

	return networkNativeID, subnetworkNativeID, clusterName, clusterNativeID
}

// TestNodePoolCreate tests the creation, reading, and deletion of a GKE NodePool
func TestNodePoolCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	nodePool, _ := NewContainerProvisioner(testutil.Config, NodePoolResourceType)

	pollConfig := testutil.DefaultPollConfig()
	pollConfig.MaxAttempts = 100
	pollConfig.CheckInterval = 10 * time.Second

	// Setup cluster infrastructure
	// For faster debugging, set createNew=false and provide existing resource names:
	//networkNativeID, subnetworkNativeID, clusterName, clusterNativeID := setupClusterForNodePool(t, ctx, false, "formae-test-np-net-fe7b7ad5", "formae-test-np-subnet-5d71495a", "formae-test-np-cluster-31cd7512")
	networkNativeID, subnetworkNativeID, clusterName, clusterNativeID := setupClusterForNodePool(t, ctx, true, "", "", "")
	// Generate unique node pool name
	nodePoolName := fmt.Sprintf("formae-test-pool-%s", uuid.New().String()[:8])
	t.Logf("Creating test node pool: %s", nodePoolName)

	// Test NodePool Create
	t.Run("CreateNodePool", func(t *testing.T) {
		nodePoolProperties := map[string]interface{}{
			"name":             nodePoolName,
			"cluster":          clusterName,
			"location":         testutil.Region,
			"initialNodeCount": 2,
			"config": map[string]interface{}{
				"machineType": "e2-small",
				"diskSizeGb":  30,
				"diskType":    "pd-standard",
				"oauthScopes": []string{
					"https://www.googleapis.com/auth/cloud-platform",
				},
				"labels": map[string]string{
					"pool-type":  "test",
					"managed-by": "formae",
				},
			},
			"management": map[string]interface{}{
				"autoRepair":  true,
				"autoUpgrade": true,
			},
			"autoscaling": map[string]interface{}{
				"enabled":      true,
				"minNodeCount": 1,
				"maxNodeCount": 3,
			},
		}

		nodePoolPropsJSON, err := json.Marshal(nodePoolProperties)
		require.NoError(t, err)

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Container::NodePool",
			Properties:   nodePoolPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		// Create the node pool
		createResult, err := nodePool.Create(ctx, createReq)
		require.NoError(t, err)
		require.NotNil(t, createResult)
		require.NotNil(t, createResult.ProgressResult)

		assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation)
		assert.Equal(t, resource.OperationStatusInProgress, createResult.ProgressResult.OperationStatus)
		require.NotEmpty(t, createResult.ProgressResult.RequestID)
		require.NotEmpty(t, createResult.ProgressResult.NativeID)

		t.Logf("NodePool creation initiated with RequestID: %s", createResult.ProgressResult.RequestID)

		// Wait for creation to complete
		statusResult, err := testutil.WaitForCreateWithConfig(t, ctx, nodePool, createResult, testutil.TargetConfig, "GCP::Container::NodePool", pollConfig)
		require.NoError(t, err)
		require.NotNil(t, statusResult)

		nativeID := statusResult.ProgressResult.NativeID
		t.Logf("NodePool created with native ID: %s", nativeID)

		// Test Read operation
		t.Run("Read", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Container::NodePool",
			}

			readResult, err := nodePool.Read(ctx, readReq)
			require.NoError(t, err)
			require.NotNil(t, readResult)
			require.Empty(t, readResult.ErrorCode)
			require.NotEmpty(t, readResult.Properties)

			// Verify properties
			var readProps map[string]interface{}
			err = json.Unmarshal([]byte(readResult.Properties), &readProps)
			require.NoError(t, err)

			assert.Equal(t, nodePoolName, utils.GetString(readProps, "name"))
			assert.Equal(t, clusterName, utils.GetString(readProps, "cluster"))
			assert.Equal(t, testutil.Region, utils.GetString(readProps, "location"))
			assert.Equal(t, "RUNNING", utils.GetString(readProps, "status"))

			// Verify node config
			nodeConfig := utils.GetObject(readProps, "config")
			require.NotNil(t, nodeConfig)
			assert.Equal(t, "e2-small", utils.GetString(nodeConfig, "machineType"))

			// Verify autoscaling
			autoscaling := utils.GetObject(readProps, "autoscaling")
			require.NotNil(t, autoscaling)
			assert.True(t, utils.GetBool(autoscaling, "enabled"))
			assert.Equal(t, int32(1), utils.GetInt32(autoscaling, "minNodeCount"))
			assert.Equal(t, int32(3), utils.GetInt32(autoscaling, "maxNodeCount"))

			// Verify management
			management := utils.GetObject(readProps, "management")
			require.NotNil(t, management)
			assert.True(t, utils.GetBool(management, "autoRepair"))
			assert.True(t, utils.GetBool(management, "autoUpgrade"))

			t.Logf("Read node pool properties successfully")
		})

		// Test List operation
		t.Run("List", func(t *testing.T) {
			listReq := &resource.ListRequest{
				ResourceType: "GCP::Container::NodePool",
				TargetConfig: testutil.TargetConfig,
				AdditionalProperties: map[string]string{
					"cluster":  clusterName,
					"location": testutil.Region,
				},
			}

			listResult, err := nodePool.List(ctx, listReq)
			require.NoError(t, err, "List operation should not return error")
			require.NotNil(t, listResult, "List result should not be nil")
			require.NotNil(t, listResult.NativeIDs, "NativeIDs list should not be nil")

			t.Logf("Found %d node pools in cluster %s", len(listResult.NativeIDs), clusterName)

			// Verify our node pool is in the list by checking native IDs
			found := false
			for _, id := range listResult.NativeIDs {
				if id == nativeID {
					found = true
					break
				}
			}
			assert.True(t, found, "Created node pool should be in the list")
		})

		// Test Delete operation
		t.Run("Delete", func(t *testing.T) {
			deleteReq := &resource.DeleteRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Container::NodePool",
			}

			deleteResult, err := nodePool.Delete(ctx, deleteReq)
			require.NoError(t, err)
			require.NotNil(t, deleteResult)
			require.NotNil(t, deleteResult.ProgressResult)

			assert.Equal(t, resource.OperationDelete, deleteResult.ProgressResult.Operation)
			assert.Equal(t, resource.OperationStatusInProgress, deleteResult.ProgressResult.OperationStatus)
			require.NotEmpty(t, deleteResult.ProgressResult.RequestID)

			t.Logf("NodePool deletion initiated with RequestID: %s", deleteResult.ProgressResult.RequestID)

			// Wait for deletion to complete
			_, err = testutil.WaitForDeleteWithConfig(t, ctx, nodePool, deleteResult, testutil.TargetConfig, "GCP::Container::NodePool", pollConfig)
			require.NoError(t, err)

			t.Logf("NodePool deleted successfully")
		})

		// Verify deletion
		t.Run("VerifyDeleted", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Container::NodePool",
			}

			readResult, err := nodePool.Read(ctx, readReq)
			require.NoError(t, err)
			require.NotNil(t, readResult)
			assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode)

			t.Logf("Verified node pool was deleted")
		})
	})

	// Cleanup cluster
	t.Run("CleanupCluster", func(t *testing.T) {
		cluster, _ := NewContainerProvisioner(testutil.Config, ClusterResourceType)

		deleteReq := &resource.DeleteRequest{
			NativeID:     clusterNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Container::Cluster",
		}

		deleteResult, err := cluster.Delete(ctx, deleteReq)
		require.NoError(t, err)

		_, err = testutil.WaitForDeleteWithConfig(t, ctx, cluster, deleteResult, testutil.TargetConfig, "GCP::Container::Cluster", pollConfig)
		require.NoError(t, err)

		t.Logf("Cluster deleted: %s", clusterName)
	})

	// Cleanup subnetwork
	t.Run("CleanupSubnetwork", func(t *testing.T) {
		subnetwork, _ := compute.NewComputeProvisioner(testutil.Config, compute.SubnetworkResourceType)

		deleteReq := &resource.DeleteRequest{
			NativeID:     subnetworkNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Compute::Subnetwork",
		}

		deleteResult, err := subnetwork.Delete(ctx, deleteReq)
		require.NoError(t, err)

		_, err = testutil.WaitForDelete(t, ctx, subnetwork, deleteResult, testutil.TargetConfig, "GCP::Compute::Subnetwork")
		require.NoError(t, err)

		t.Logf("Subnetwork deleted: %s", subnetworkNativeID)
	})

	// Cleanup network
	t.Run("CleanupNetwork", func(t *testing.T) {
		network, _ := compute.NewComputeProvisioner(testutil.Config, compute.NetworkResourceType)

		deleteReq := &resource.DeleteRequest{
			NativeID:     networkNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Compute::Network",
		}

		deleteResult, err := network.Delete(ctx, deleteReq)
		require.NoError(t, err)

		_, err = testutil.WaitForDelete(t, ctx, network, deleteResult, testutil.TargetConfig, "GCP::Compute::Network")
		require.NoError(t, err)

		t.Logf("Network deleted: %s", networkNativeID)
	})
}

// TestNodePoolReadNonExistent tests reading a non-existent node pool
func TestNodePoolReadNonExistent(t *testing.T) {
	nodePool, _ := NewContainerProvisioner(testutil.Config, NodePoolResourceType)
	ctx := context.Background()

	// Use a non-existent node pool
	nonExistentCluster := "non-existent-cluster-" + uuid.New().String()[:8]
	nonExistentNodePool := "non-existent-pool-" + uuid.New().String()[:8]
	nativeID := BuildNodePoolPath(testutil.Project, testutil.Region, nonExistentCluster, nonExistentNodePool)

	t.Logf("Attempting to read non-existent node pool: %s", nativeID)

	readReq := &resource.ReadRequest{
		NativeID:     nativeID,
		TargetConfig: testutil.TargetConfig,
		ResourceType: "GCP::Container::NodePool",
	}

	readResult, err := nodePool.Read(ctx, readReq)
	require.NoError(t, err)
	require.NotNil(t, readResult)

	// Verify that we get a NotFound error code
	assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode)
	assert.Empty(t, readResult.Properties)

	t.Logf("✓ Non-existent node pool correctly returns NotFound")
}

// TestNodePoolReadNotFound tests that reading a non-existent node pool returns NotFound
func TestNodePoolReadNotFound(t *testing.T) {
	nodePool, _ := NewContainerProvisioner(testutil.Config, NodePoolResourceType)
	ctx := context.Background()

	t.Run("NonExistentNodePool", func(t *testing.T) {
		// Generate a random non-existent node pool name
		nonExistentCluster := "non-existent-cluster-" + uuid.New().String()[:8]
		nonExistentNodePool := "non-existent-pool-" + uuid.New().String()[:8]
		nativeID := BuildNodePoolPath(testutil.Project, testutil.Region, nonExistentCluster, nonExistentNodePool)

		t.Logf("Attempting to read non-existent node pool: %s", nativeID)

		readReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Container::NodePool",
		}

		readResult, err := nodePool.Read(ctx, readReq)
		require.NoError(t, err, "Read operation should not return error")
		require.NotNil(t, readResult, "Read result should not be nil")

		// Verify that we get a NotFound error code
		assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode,
			"Reading non-existent node pool should return NotFound error code")
		assert.Empty(t, readResult.Properties,
			"Properties should be empty for non-existent node pool")

		t.Logf("✓ Non-existent node pool correctly returns NotFound error code")
	})

	t.Run("NonExistentNodePoolInExistingCluster", func(t *testing.T) {
		// First, find an existing cluster
		cluster, _ := NewContainerProvisioner(testutil.Config, ClusterResourceType)

		listReq := &resource.ListRequest{
			ResourceType: "GCP::Container::Cluster",
			TargetConfig: testutil.TargetConfig,
		}

		listResult, err := cluster.List(ctx, listReq)
		require.NoError(t, err)

		if len(listResult.NativeIDs) == 0 {
			t.Skip("No clusters found to test non-existent node pool in existing cluster")
		}

		// Get first cluster by reading it
		readReq := &resource.ReadRequest{
			NativeID:     listResult.NativeIDs[0],
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Container::Cluster",
		}
		readResult, err := cluster.Read(ctx, readReq)
		require.NoError(t, err)

		var clusterProps map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &clusterProps)
		require.NoError(t, err)

		existingClusterName := utils.GetString(clusterProps, "name")
		location := utils.GetString(clusterProps, "location")

		// Generate a non-existent node pool name for this existing cluster
		nonExistentNodePool := "non-existent-pool-" + uuid.New().String()[:8]
		nativeID := BuildNodePoolPath(testutil.Project, location, existingClusterName, nonExistentNodePool)

		t.Logf("Attempting to read non-existent node pool '%s' in existing cluster '%s'",
			nonExistentNodePool, existingClusterName)

		nodePoolReadReq := &resource.ReadRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Container::NodePool",
		}

		nodePoolReadResult, err := nodePool.Read(ctx, nodePoolReadReq)
		require.NoError(t, err, "Read operation should not return error")
		require.NotNil(t, nodePoolReadResult, "Read result should not be nil")

		// Verify that we get a NotFound error code
		assert.Equal(t, resource.OperationErrorCodeNotFound, nodePoolReadResult.ErrorCode,
			"Reading non-existent node pool in existing cluster should return NotFound error code")
		assert.Empty(t, nodePoolReadResult.Properties,
			"Properties should be empty for non-existent node pool")

		t.Logf("✓ Non-existent node pool in existing cluster correctly returns NotFound error code")
	})

	t.Run("InvalidNodePoolPath", func(t *testing.T) {
		// Test with invalid path format
		invalidNativeID := "invalid-path-format"

		t.Logf("Attempting to read with invalid native ID: %s", invalidNativeID)

		readReq := &resource.ReadRequest{
			NativeID:     invalidNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Container::NodePool",
		}

		readResult, err := nodePool.Read(ctx, readReq)

		// Should either return an error or a NotFound result
		if err != nil {
			t.Logf("✓ Invalid native ID correctly returns error: %v", err)
			assert.Contains(t, err.Error(), "invalid", "Error should mention invalid format")
		} else {
			require.NotNil(t, readResult)
			assert.NotEmpty(t, readResult.ErrorCode, "Should have an error code")
			t.Logf("✓ Invalid native ID correctly returns error code: %s", readResult.ErrorCode)
		}
	})

	t.Run("DeleteNonExistentNodePool", func(t *testing.T) {
		// Test deleting a non-existent node pool
		nonExistentCluster := "non-existent-cluster-" + uuid.New().String()[:8]
		nonExistentNodePool := "non-existent-pool-" + uuid.New().String()[:8]
		nativeID := BuildNodePoolPath(testutil.Project, testutil.Region, nonExistentCluster, nonExistentNodePool)

		t.Logf("Attempting to delete non-existent node pool: %s", nativeID)

		deleteReq := &resource.DeleteRequest{
			NativeID:     nativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Container::NodePool",
		}

		deleteResult, err := nodePool.Delete(ctx, deleteReq)
		require.NoError(t, err, "Delete operation should not return error")
		require.NotNil(t, deleteResult, "Delete result should not be nil")
		require.NotNil(t, deleteResult.ProgressResult, "Progress result should not be nil")

		// Deleting a non-existent resource should succeed (idempotent)
		assert.Equal(t, resource.OperationDelete, deleteResult.ProgressResult.Operation)
		assert.Equal(t, resource.OperationStatusSuccess, deleteResult.ProgressResult.OperationStatus,
			"Deleting non-existent node pool should succeed (idempotent)")

		t.Logf("✓ Delete non-existent node pool correctly returns success (idempotent)")
	})
}

func TestNodePoolPathParsing(t *testing.T) {
	t.Run("BuildAndParseNodePoolPath", func(t *testing.T) {
		project := "test-project"
		location := "us-central1"
		cluster := "test-cluster"
		nodePool := "test-pool"

		// Build path
		nativeID := BuildNodePoolPath(project, location, cluster, nodePool)
		expectedPath := "projects/test-project/locations/us-central1/clusters/test-cluster/nodePools/test-pool"
		assert.Equal(t, expectedPath, nativeID, "Built path should match expected format")

		// Parse path
		parsedProject, parsedLocation, parsedCluster, parsedNodePool, err := ParseNodePoolPath(nativeID)
		require.NoError(t, err, "Parsing should succeed")
		assert.Equal(t, project, parsedProject, "Parsed project should match")
		assert.Equal(t, location, parsedLocation, "Parsed location should match")
		assert.Equal(t, cluster, parsedCluster, "Parsed cluster should match")
		assert.Equal(t, nodePool, parsedNodePool, "Parsed node pool should match")
	})

	t.Run("ParseInvalidNodePoolPath", func(t *testing.T) {
		invalidPaths := []string{
			"",
			"invalid",
			"projects/test/locations/us-central1/clusters/test",
			"projects/test/locations/us-central1/clusters/test/nodePools",
			"projects/test/regions/us-central1/clusters/test/nodePools/pool", // wrong format (regions instead of locations)
		}

		for _, invalidPath := range invalidPaths {
			_, _, _, _, err := ParseNodePoolPath(invalidPath)
			assert.Error(t, err, "Should return error for invalid path: %s", invalidPath)
		}
	})

	t.Run("NodePoolPathComponents", func(t *testing.T) {
		nativeID := "projects/test-project/locations/us-central1/clusters/test-cluster/nodePools/test-pool"

		components, err := NewNodePoolPathComponents(nativeID)
		require.NoError(t, err, "Should parse components")
		require.NotNil(t, components, "Components should not be nil")

		assert.Equal(t, "test-project", components.Project)
		assert.Equal(t, "us-central1", components.Location)
		assert.Equal(t, "test-cluster", components.ClusterName)
		assert.Equal(t, "test-pool", components.NodePoolName)

		// Test ToNativeID
		rebuiltID := components.ToNativeID()
		assert.Equal(t, nativeID, rebuiltID, "Rebuilt ID should match original")

		expectedClusterPath := "projects/test-project/locations/us-central1/clusters/test-cluster"

		// Test ToClusterNativeID
		clusterNativeID := components.ToClusterNativeID()
		assert.Equal(t, expectedClusterPath, clusterNativeID, "Cluster native ID should match")

		// Test ToOperationPath
		operationPath := components.ToOperationPath("operation-123")
		expectedOpPath := "projects/test-project/locations/us-central1/operations/operation-123"
		assert.Equal(t, expectedOpPath, operationPath, "Operation path should be correct")
	})
}

// TestNodePoolList tests listing node pools across all clusters
func TestNodePoolList(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	nodePool, _ := NewContainerProvisioner(testutil.Config, NodePoolResourceType)
	ctx := context.Background()
	clusterName := "formae-test-np-cluster-17064fcc"
	t.Run("ListAllNodePools", func(t *testing.T) {
		listReq := &resource.ListRequest{
			ResourceType: "GCP::Container::NodePool",
			TargetConfig: testutil.TargetConfig,
			AdditionalProperties: map[string]string{
				"cluster":  clusterName,
				"location": testutil.Region,
			},
		}

		listResult, err := nodePool.List(ctx, listReq)
		require.NoError(t, err, "List operation should not return error")
		require.NotNil(t, listResult, "List result should not be nil")
		require.NotNil(t, listResult.NativeIDs, "NativeIDs list should not be nil")

		t.Logf("Found %d node pool(s) in region %s", len(listResult.NativeIDs), testutil.Region)

		if len(listResult.NativeIDs) == 0 {
			t.Log("No node pools found (this is okay if no clusters exist)")
			return
		}

		// Test reading details of each node pool
		for i, nativeID := range listResult.NativeIDs {
			// Verify native ID format
			assert.Contains(t, nativeID, "projects/")
			assert.Contains(t, nativeID, "locations/")
			assert.Contains(t, nativeID, "clusters/")
			assert.Contains(t, nativeID, "nodePools/")

			// Read the node pool to get its properties
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Container::NodePool",
			}

			readResult, err := nodePool.Read(ctx, readReq)
			require.NoError(t, err, "Read operation should not return error")
			require.NotNil(t, readResult, "Read result should not be nil")
			require.Empty(t, readResult.ErrorCode, "Read should not have error code")
			require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

			var props map[string]interface{}
			err = json.Unmarshal([]byte(readResult.Properties), &props)
			require.NoError(t, err, "Failed to unmarshal node pool properties")

			poolName := utils.GetString(props, "name")
			poolClusterName := utils.GetString(props, "cluster")
			location := utils.GetString(props, "location")
			status := utils.GetString(props, "status")
			machineType := utils.GetString(props, "machineType")

			t.Logf("NodePool %d:", i+1)
			t.Logf("  Name: %s", poolName)
			t.Logf("  Cluster: %s", poolClusterName)
			t.Logf("  Location: %s", location)
			t.Logf("  Status: %s", status)
			t.Logf("  Machine Type: %s", machineType)
			t.Logf("  Native ID: %s", nativeID)

			// Check autoscaling info
			if autoscaling := utils.GetObject(props, "autoscaling"); autoscaling != nil {
				if utils.GetBool(autoscaling, "enabled") {
					t.Logf("  Autoscaling: %d - %d nodes",
						utils.GetInt32(autoscaling, "minNodeCount"),
						utils.GetInt32(autoscaling, "maxNodeCount"))
				}
			}

			t.Logf("  ✓ Successfully read full details for node pool: %s", poolName)

			// Only test first 3 to avoid long test times
			if i >= 2 {
				break
			}
		}
	})
}

// TestNodePoolListWithFilter tests listing node pools for a specific cluster
func TestNodePoolListWithFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	nodePool, _ := NewContainerProvisioner(testutil.Config, NodePoolResourceType)
	cluster, _ := NewContainerProvisioner(testutil.Config, ClusterResourceType)
	ctx := context.Background()

	// First, list clusters to find one
	t.Run("FindClusterAndListNodePools", func(t *testing.T) {
		clusterListReq := &resource.ListRequest{
			ResourceType: "GCP::Container::Cluster",
			TargetConfig: testutil.TargetConfig,
		}

		clusterListResult, err := cluster.List(ctx, clusterListReq)
		require.NoError(t, err)

		if len(clusterListResult.NativeIDs) == 0 {
			t.Skip("No clusters found to test node pool listing")
		}

		// Get first cluster by reading it
		clusterReadReq := &resource.ReadRequest{
			NativeID:     clusterListResult.NativeIDs[0],
			TargetConfig: testutil.TargetConfig,
			ResourceType: "GCP::Container::Cluster",
		}
		clusterReadResult, err := cluster.Read(ctx, clusterReadReq)
		require.NoError(t, err)

		var clusterProps map[string]interface{}
		err = json.Unmarshal([]byte(clusterReadResult.Properties), &clusterProps)
		require.NoError(t, err)

		clusterName := utils.GetString(clusterProps, "name")
		location := utils.GetString(clusterProps, "location")

		t.Logf("Testing node pool listing for cluster: %s", clusterName)

		// List all node pools
		listReq := &resource.ListRequest{
			ResourceType: "GCP::Container::NodePool",
			TargetConfig: testutil.TargetConfig,
		}

		listResult, err := nodePool.List(ctx, listReq)
		require.NoError(t, err)

		// Filter results for this specific cluster by reading each and checking
		var clusterNodePoolIDs []string
		for _, nativeID := range listResult.NativeIDs {
			// Parse the native ID to check if it belongs to this cluster
			components, err := NewNodePoolPathComponents(nativeID)
			if err == nil && components.ClusterName == clusterName {
				clusterNodePoolIDs = append(clusterNodePoolIDs, nativeID)
			}
		}

		t.Logf("Found %d node pool(s) for cluster %s", len(clusterNodePoolIDs), clusterName)

		for i, nativeID := range clusterNodePoolIDs {
			// Read each node pool to get its properties
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Container::NodePool",
			}
			readResult, err := nodePool.Read(ctx, readReq)
			require.NoError(t, err)

			var props map[string]interface{}
			err = json.Unmarshal([]byte(readResult.Properties), &props)
			require.NoError(t, err)

			t.Logf("  Pool %d: %s (status: %s)",
				i+1,
				utils.GetString(props, "name"),
				utils.GetString(props, "status"))

			// Verify the native ID matches the cluster
			components, err := NewNodePoolPathComponents(nativeID)
			require.NoError(t, err)
			assert.Equal(t, clusterName, components.ClusterName)
			assert.Equal(t, location, components.Location)
		}
	})
}

// ...existing code...
