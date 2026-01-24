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

// TestClusterWithDataplaneV2 tests creating a GKE cluster with Dataplane V2 (ADVANCED_DATAPATH)
// This configuration mirrors the gke.pkl example and verifies that:
// - networkPolicy is NOT specified (incompatible with Dataplane V2)
// - networkPolicyConfig in addonsConfig is NOT specified
// - datapathProvider = "ADVANCED_DATAPATH" works correctly
func TestClusterWithDataplaneV2(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	network, err := compute.NewComputeProvisioner(testutil.Config, compute.NetworkResourceType)
	require.NoError(t, err)
	subnetwork, err := compute.NewComputeProvisioner(testutil.Config, compute.SubnetworkResourceType)
	require.NoError(t, err)
	cluster, err := NewContainerProvisioner(testutil.Config, ClusterResourceType)
	require.NoError(t, err)

	// Generate unique names
	suffix := uuid.New().String()[:8]
	networkName := fmt.Sprintf("formae-test-dp2-net-%s", suffix)
	subnetworkName := fmt.Sprintf("formae-test-dp2-subnet-%s", suffix)
	clusterName := fmt.Sprintf("formae-test-dp2-%s", suffix)

	t.Logf("Creating test resources with suffix: %s", suffix)
	t.Logf("Network: %s", networkName)
	t.Logf("Subnetwork: %s", subnetworkName)
	t.Logf("Cluster: %s", clusterName)

	var networkNativeID string
	var subnetworkNativeID string
	var networkSelfLink string
	var subnetworkSelfLink string

	// Create network
	t.Run("SetupNetwork", func(t *testing.T) {
		networkProperties := map[string]interface{}{
			"name":                  networkName,
			"description":           "VPC network for GKE cluster with Dataplane V2",
			"autoCreateSubnetworks": false,
			"routingConfig": map[string]interface{}{
				"routingMode": "REGIONAL",
			},
			"mtu": 1460,
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
		require.NotEmpty(t, networkSelfLink, "Network selfLink should not be empty")

		t.Logf("Network created: %s (selfLink: %s)", networkName, networkSelfLink)
	})

	// Create subnetwork with secondary ranges for pods and services
	t.Run("SetupSubnetwork", func(t *testing.T) {
		subnetworkProperties := map[string]interface{}{
			"name":        subnetworkName,
			"description": "Subnet for GKE cluster with Dataplane V2",
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
		var subnetworkProps map[string]interface{}
		err = json.Unmarshal(statusResult.ProgressResult.ResourceProperties, &subnetworkProps)
		require.NoError(t, err)
		subnetworkSelfLink = utils.GetString(subnetworkProps, "selfLink")

		t.Logf("Subnetwork created: %s (selfLink: %s)", subnetworkName, subnetworkSelfLink)
	})

	// Create GKE cluster with Dataplane V2 configuration (mirrors gke.pkl)
	t.Run("CreateClusterWithDataplaneV2", func(t *testing.T) {
		// This configuration mirrors gke.pkl exactly
		// IMPORTANT: networkPolicy and addonsConfig.networkPolicyConfig are NOT specified
		// because they are incompatible with datapathProvider = "ADVANCED_DATAPATH"
		clusterProperties := map[string]interface{}{
			"name":             clusterName,
			"description":      "Formae test GKE cluster with Dataplane V2",
			"location":         testutil.Region,
			"network":          networkSelfLink,
			"subnetwork":       subnetworkSelfLink,
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
				"tags": []string{"gke-node"},
				"workloadMetadataConfig": map[string]interface{}{
					"mode": "GKE_METADATA",
				},
			},
			"ipAllocationPolicy": map[string]interface{}{
				"useIpAliases":               true,
				"clusterSecondaryRangeName":  "pods",
				"servicesSecondaryRangeName": "services",
				"stackType":                  "IPV4",
			},
			"privateClusterConfig": map[string]interface{}{
				"enablePrivateNodes":    true,
				"enablePrivateEndpoint": false,
				"masterIpv4CidrBlock":   "172.16.0.0/28",
				"masterGlobalAccessConfig": map[string]interface{}{
					"enabled": true,
				},
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
			// Note: networkPolicyConfig is NOT specified - incompatible with ADVANCED_DATAPATH
			"addonsConfig": map[string]interface{}{
				"httpLoadBalancing": map[string]interface{}{
					"disabled": false,
				},
				"horizontalPodAutoscaling": map[string]interface{}{
					"disabled": false,
				},
				"gcePersistentDiskCsiDriverConfig": map[string]interface{}{
					"enabled": true,
				},
				"gcpFilestoreCsiDriverConfig": map[string]interface{}{
					"enabled": false,
				},
				"gcsFuseCsiDriverConfig": map[string]interface{}{
					"enabled": false,
				},
			},
			"networkConfig": map[string]interface{}{
				"enableIntraNodeVisibility": true,
				"datapathProvider":          "ADVANCED_DATAPATH", // GKE Dataplane V2
				"enableL4ilbSubsetting":     true,
				"dnsConfig": map[string]interface{}{
					"clusterDns":      "CLOUD_DNS",
					"clusterDnsScope": "VPC_SCOPE",
				},
			},
			"releaseChannel": map[string]interface{}{
				"channel": "REGULAR",
			},
			"workloadIdentityConfig": map[string]interface{}{
				"workloadPool": fmt.Sprintf("%s.svc.id.goog", testutil.Project),
			},
			"loggingConfig": map[string]interface{}{
				"componentConfig": map[string]interface{}{
					"enableComponents": []string{"SYSTEM_COMPONENTS", "WORKLOADS"},
				},
			},
			"monitoringConfig": map[string]interface{}{
				"componentConfig": map[string]interface{}{
					"enableComponents": []string{"SYSTEM_COMPONENTS", "WORKLOADS"},
				},
				"managedPrometheusConfig": map[string]interface{}{
					"enabled": true,
				},
			},
			"binaryAuthorization": map[string]interface{}{
				"evaluationMode": "DISABLED",
			},
			"verticalPodAutoscaling": map[string]interface{}{
				"enabled": true,
			},
			// Note: networkPolicy is NOT specified - incompatible with ADVANCED_DATAPATH
			// GKE Dataplane V2 has network policy enforcement built-in
			"resourceLabels": map[string]string{
				"team":       "platform",
				"env":        "test",
				"managed-by": "formae",
			},
			"maintenancePolicy": map[string]interface{}{
				"window": map[string]interface{}{
					"dailyMaintenanceWindow": map[string]interface{}{
						"startTime": "03:00",
					},
				},
			},
		}

		clusterPropsJSON, err := json.Marshal(clusterProperties)
		require.NoError(t, err)

		t.Logf("Creating cluster with properties:\n%s", string(clusterPropsJSON))

		createReq := &resource.CreateRequest{
			ResourceType: "GCP::Container::Cluster",
			Properties:   clusterPropsJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := cluster.Create(ctx, createReq)
		require.NoError(t, err, "Create operation should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation)
		if createResult.ProgressResult.OperationStatus == resource.OperationStatusFailure {
			t.Fatalf("Cluster creation failed: %s (error code: %s)",
				createResult.ProgressResult.StatusMessage,
				createResult.ProgressResult.ErrorCode)
		}
		assert.Equal(t, resource.OperationStatusInProgress, createResult.ProgressResult.OperationStatus)
		require.NotEmpty(t, createResult.ProgressResult.RequestID)

		t.Logf("Cluster creation initiated with RequestID: %s", createResult.ProgressResult.RequestID)

		// Wait for creation (clusters take 5-15 minutes)
		pollConfig := testutil.DefaultPollConfig()
		pollConfig.MaxAttempts = 120
		pollConfig.CheckInterval = 15 * time.Second

		statusResult, err := testutil.WaitForCreateWithConfig(
			t, ctx, cluster, createResult,
			testutil.TargetConfig, "GCP::Container::Cluster", pollConfig)
		require.NoError(t, err, "Cluster creation should complete successfully")
		require.NotNil(t, statusResult)

		nativeID := statusResult.ProgressResult.NativeID
		t.Logf("Cluster created with native ID: %s", nativeID)

		// Verify cluster properties
		t.Run("VerifyClusterProperties", func(t *testing.T) {
			readReq := &resource.ReadRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Container::Cluster",
			}

			readResult, err := cluster.Read(ctx, readReq)
			require.NoError(t, err)
			require.Empty(t, readResult.ErrorCode)

			var readProps map[string]interface{}
			err = json.Unmarshal([]byte(readResult.Properties), &readProps)
			require.NoError(t, err)

			// Verify basic properties
			assert.Equal(t, clusterName, utils.GetString(readProps, "name"))
			assert.Equal(t, testutil.Region, utils.GetString(readProps, "location"))
			assert.Equal(t, "RUNNING", utils.GetString(readProps, "status"))

			// Verify Dataplane V2 is enabled
			networkConfig := utils.GetObject(readProps, "networkConfig")
			require.NotNil(t, networkConfig, "Network config should exist")
			assert.Equal(t, "ADVANCED_DATAPATH", utils.GetString(networkConfig, "datapathProvider"),
				"Dataplane V2 should be enabled")

			// Verify private cluster config
			privateClusterConfig := utils.GetObject(readProps, "privateClusterConfig")
			require.NotNil(t, privateClusterConfig)
			assert.True(t, utils.GetBool(privateClusterConfig, "enablePrivateNodes"))
			assert.NotEmpty(t, utils.GetString(privateClusterConfig, "privateEndpoint"))

			// Verify workload identity
			workloadIdentityConfig := utils.GetObject(readProps, "workloadIdentityConfig")
			require.NotNil(t, workloadIdentityConfig)
			expectedWorkloadPool := fmt.Sprintf("%s.svc.id.goog", testutil.Project)
			assert.Equal(t, expectedWorkloadPool, utils.GetString(workloadIdentityConfig, "workloadPool"))

			// Verify resource labels
			resourceLabels := utils.GetObject(readProps, "resourceLabels")
			require.NotNil(t, resourceLabels)
			assert.Equal(t, "platform", utils.GetString(resourceLabels, "team"))
			assert.Equal(t, "test", utils.GetString(resourceLabels, "env"))

			t.Logf("Cluster properties verified successfully")
		})

		// Delete cluster
		t.Run("DeleteCluster", func(t *testing.T) {
			deleteReq := &resource.DeleteRequest{
				NativeID:     nativeID,
				TargetConfig: testutil.TargetConfig,
				ResourceType: "GCP::Container::Cluster",
			}

			deleteResult, err := cluster.Delete(ctx, deleteReq)
			require.NoError(t, err)

			_, err = testutil.WaitForDeleteWithConfig(t, ctx, cluster, deleteResult,
				testutil.TargetConfig, "GCP::Container::Cluster", pollConfig)
			require.NoError(t, err)

			t.Logf("Cluster deleted successfully")
		})
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
		t.Logf("Subnetwork deleted: %s", subnetworkNativeID)
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
		t.Logf("Network deleted: %s", networkNativeID)
	})
}
