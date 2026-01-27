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
	"time"

	"github.com/google/uuid"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/testutil"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestForwardingRuleCreate tests the creation, reading, and deletion of a regional GCP Forwarding Rule
func TestForwardingRuleCreate(t *testing.T) {
	testID := uuid.New().String()[:8]

	// Create provisioners
	targetPoolProvisioner, err := NewComputeProvisioner(testutil.Config, TargetPoolResourceType)
	require.NoError(t, err, "Failed to create target pool provisioner")

	forwardingRuleProvisioner, err := NewComputeProvisioner(testutil.Config, ForwardingRuleResourceType)
	require.NoError(t, err, "Failed to create forwarding rule provisioner")

	ctx := context.Background()

	// First, create a target pool (required dependency for classic network LB)
	targetPoolName := fmt.Sprintf("formae-test-tp-%s", testID)
	t.Logf("Creating prerequisite target pool: %s", targetPoolName)

	tpProperties := map[string]interface{}{
		"name":        targetPoolName,
		"region":      testutil.Region,
		"description": "Target pool for forwarding rule test",
	}

	tpPropertiesJSON, err := json.Marshal(tpProperties)
	require.NoError(t, err, "Failed to marshal target pool properties")

	tpCreateReq := &resource.CreateRequest{
		ResourceType: TargetPoolResourceType,
		Properties:   tpPropertiesJSON,
		TargetConfig: testutil.TargetConfig,
	}

	tpCreateResult, err := targetPoolProvisioner.Create(ctx, tpCreateReq)
	require.NoError(t, err, "Target pool creation should not return error")

	tpStatusResult, err := testutil.WaitForCreate(t, ctx, targetPoolProvisioner, tpCreateResult, testutil.TargetConfig, TargetPoolResourceType)
	require.NoError(t, err, "Target pool creation should complete successfully")

	targetPoolNativeID := tpStatusResult.ProgressResult.NativeID
	targetPoolSelfLink := fmt.Sprintf("https://www.googleapis.com/compute/v1/%s", targetPoolNativeID)
	t.Logf("Target pool created: %s", targetPoolNativeID)

	// Cleanup target pool at the end
	defer func() {
		t.Log("Cleaning up target pool...")
		deleteReq := &resource.DeleteRequest{
			NativeID:     targetPoolNativeID,
			TargetConfig: testutil.TargetConfig,
		}
		deleteResult, err := targetPoolProvisioner.Delete(ctx, deleteReq)
		if err != nil {
			t.Logf("Warning: Failed to delete target pool: %v", err)
			return
		}
		_, err = testutil.WaitForDelete(t, ctx, targetPoolProvisioner, deleteResult, testutil.TargetConfig, TargetPoolResourceType)
		if err != nil {
			t.Logf("Warning: Target pool deletion did not complete: %v", err)
		}
	}()

	// Now create the forwarding rule
	forwardingRuleName := fmt.Sprintf("formae-test-fr-%s", testID)
	t.Logf("Creating test forwarding rule: %s", forwardingRuleName)

	var forwardingRuleNativeID string

	// Test Create operation
	t.Run("Create", func(t *testing.T) {
		frProperties := map[string]interface{}{
			"name":                forwardingRuleName,
			"region":              testutil.Region,
			"description":         "Test forwarding rule created by Formae integration test",
			"IPProtocol":          "TCP",
			"portRange":           "80-80",
			"target":              targetPoolSelfLink,
			"loadBalancingScheme": "EXTERNAL",
		}

		frPropertiesJSON, err := json.Marshal(frProperties)
		require.NoError(t, err, "Failed to marshal forwarding rule properties")

		createReq := &resource.CreateRequest{
			ResourceType: ForwardingRuleResourceType,
			Properties:   frPropertiesJSON,
			TargetConfig: testutil.TargetConfig,
		}

		createResult, err := forwardingRuleProvisioner.Create(ctx, createReq)
		require.NoError(t, err, "Create operation should not return error")
		require.NotNil(t, createResult, "Create result should not be nil")
		require.NotNil(t, createResult.ProgressResult, "Progress result should not be nil")

		assert.Equal(t, resource.OperationCreate, createResult.ProgressResult.Operation, "Operation should be Create")
		assert.Equal(t, resource.OperationStatusInProgress, createResult.ProgressResult.OperationStatus, "Should be in progress")

		t.Logf("Forwarding rule creation initiated with RequestID: %s", createResult.ProgressResult.RequestID)

		// Wait for creation to complete
		statusResult, err := testutil.WaitForCreate(t, ctx, forwardingRuleProvisioner, createResult, testutil.TargetConfig, ForwardingRuleResourceType)
		require.NoError(t, err, "Forwarding rule creation should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")

		forwardingRuleNativeID = statusResult.ProgressResult.NativeID
		t.Logf("Forwarding rule created with native ID: %s", forwardingRuleNativeID)

		// Verify native ID format
		expectedNativeID := fmt.Sprintf("projects/%s/regions/%s/forwardingRules/%s", testutil.Project, testutil.Region, forwardingRuleName)
		assert.Equal(t, expectedNativeID, forwardingRuleNativeID, "Native ID should match expected format")
	})

	// Test Read operation
	t.Run("Read", func(t *testing.T) {
		readReq := &resource.ReadRequest{
			NativeID:     forwardingRuleNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: ForwardingRuleResourceType,
		}

		readResult, err := forwardingRuleProvisioner.Read(ctx, readReq)
		require.NoError(t, err, "Read operation should not return error")
		require.NotNil(t, readResult, "Read result should not be nil")
		require.Empty(t, readResult.ErrorCode, "Read should not have error code")
		require.NotEmpty(t, readResult.Properties, "Properties should not be empty")

		// Verify properties
		var readProps map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &readProps)
		require.NoError(t, err, "Failed to unmarshal read properties")

		assert.Equal(t, forwardingRuleName, readProps["name"], "Forwarding rule name should match")
		assert.Equal(t, "Test forwarding rule created by Formae integration test", readProps["description"], "Description should match")
		assert.Equal(t, "TCP", readProps["IPProtocol"], "Protocol should match")
		assert.NotEmpty(t, readProps["IPAddress"], "Should have an allocated IP address")
		t.Logf("Read forwarding rule properties: IP=%s", readProps["IPAddress"])
	})

	// Test List operation
	t.Run("List", func(t *testing.T) {
		listReq := &resource.ListRequest{
			ResourceType: ForwardingRuleResourceType,
			TargetConfig: testutil.TargetConfig,
		}

		listResult, err := forwardingRuleProvisioner.List(ctx, listReq)
		require.NoError(t, err, "List operation should not return error")
		require.NotNil(t, listResult, "List result should not be nil")
		assert.NotEmpty(t, listResult.NativeIDs, "Resources list should not be empty")

		// Check if our test forwarding rule is in the list
		found := false
		for _, id := range listResult.NativeIDs {
			if id == forwardingRuleNativeID {
				found = true
				break
			}
		}
		assert.True(t, found, "Test forwarding rule should be in the list")
		t.Logf("Successfully found test forwarding rule in the list")
	})

	// Test Delete operation
	t.Run("Delete", func(t *testing.T) {
		deleteReq := &resource.DeleteRequest{
			NativeID:     forwardingRuleNativeID,
			TargetConfig: testutil.TargetConfig,
		}

		deleteResult, err := forwardingRuleProvisioner.Delete(ctx, deleteReq)
		require.NoError(t, err, "Delete operation should not return error")
		require.NotNil(t, deleteResult, "Delete result should not be nil")

		// Wait for deletion to complete
		statusResult, err := testutil.WaitForDelete(t, ctx, forwardingRuleProvisioner, deleteResult, testutil.TargetConfig, ForwardingRuleResourceType)
		require.NoError(t, err, "Forwarding rule deletion should complete successfully")
		require.NotNil(t, statusResult, "Status result should not be nil")

		t.Logf("Forwarding rule deleted successfully")
	})

	// Verify deletion
	t.Run("VerifyDeleted", func(t *testing.T) {
		time.Sleep(2 * time.Second)

		readReq := &resource.ReadRequest{
			NativeID:     forwardingRuleNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: ForwardingRuleResourceType,
		}

		readResult, err := forwardingRuleProvisioner.Read(ctx, readReq)

		if err == nil {
			require.NotNil(t, readResult, "Read result should not be nil")
			assert.NotEmpty(t, readResult.ErrorCode, "Should have error code for deleted resource")
		}

		t.Logf("Verified forwarding rule was deleted")
	})
}

// TestGlobalForwardingRuleCreate tests the creation and deletion of a Global Forwarding Rule
func TestGlobalForwardingRuleCreate(t *testing.T) {
	testID := uuid.New().String()[:8]

	// Create provisioners
	healthCheckProvisioner, err := NewComputeProvisioner(testutil.Config, HealthCheckResourceType)
	require.NoError(t, err, "Failed to create health check provisioner")

	backendServiceProvisioner, err := NewComputeProvisioner(testutil.Config, BackendServiceResourceType)
	require.NoError(t, err, "Failed to create backend service provisioner")

	urlMapProvisioner, err := NewComputeProvisioner(testutil.Config, UrlMapResourceType)
	require.NoError(t, err, "Failed to create url map provisioner")

	targetHttpProxyProvisioner, err := NewComputeProvisioner(testutil.Config, TargetHttpProxyResourceType)
	require.NoError(t, err, "Failed to create target http proxy provisioner")

	globalForwardingRuleProvisioner, err := NewComputeProvisioner(testutil.Config, GlobalForwardingRuleResourceType)
	require.NoError(t, err, "Failed to create global forwarding rule provisioner")

	ctx := context.Background()

	// Track resources for cleanup
	var healthCheckNativeID, backendServiceNativeID, urlMapNativeID, targetHttpProxyNativeID, globalForwardingRuleNativeID string

	// Cleanup function
	defer func() {
		t.Log("Cleaning up resources...")
		// Delete in reverse order of dependencies
		if globalForwardingRuleNativeID != "" {
			deleteReq := &resource.DeleteRequest{NativeID: globalForwardingRuleNativeID, TargetConfig: testutil.TargetConfig}
			if deleteResult, err := globalForwardingRuleProvisioner.Delete(ctx, deleteReq); err == nil {
				testutil.WaitForDelete(t, ctx, globalForwardingRuleProvisioner, deleteResult, testutil.TargetConfig, GlobalForwardingRuleResourceType)
			}
		}
		if targetHttpProxyNativeID != "" {
			deleteReq := &resource.DeleteRequest{NativeID: targetHttpProxyNativeID, TargetConfig: testutil.TargetConfig}
			if deleteResult, err := targetHttpProxyProvisioner.Delete(ctx, deleteReq); err == nil {
				testutil.WaitForDelete(t, ctx, targetHttpProxyProvisioner, deleteResult, testutil.TargetConfig, TargetHttpProxyResourceType)
			}
		}
		if urlMapNativeID != "" {
			deleteReq := &resource.DeleteRequest{NativeID: urlMapNativeID, TargetConfig: testutil.TargetConfig}
			if deleteResult, err := urlMapProvisioner.Delete(ctx, deleteReq); err == nil {
				testutil.WaitForDelete(t, ctx, urlMapProvisioner, deleteResult, testutil.TargetConfig, UrlMapResourceType)
			}
		}
		if backendServiceNativeID != "" {
			deleteReq := &resource.DeleteRequest{NativeID: backendServiceNativeID, TargetConfig: testutil.TargetConfig}
			if deleteResult, err := backendServiceProvisioner.Delete(ctx, deleteReq); err == nil {
				testutil.WaitForDelete(t, ctx, backendServiceProvisioner, deleteResult, testutil.TargetConfig, BackendServiceResourceType)
			}
		}
		if healthCheckNativeID != "" {
			deleteReq := &resource.DeleteRequest{NativeID: healthCheckNativeID, TargetConfig: testutil.TargetConfig}
			if deleteResult, err := healthCheckProvisioner.Delete(ctx, deleteReq); err == nil {
				testutil.WaitForDelete(t, ctx, healthCheckProvisioner, deleteResult, testutil.TargetConfig, HealthCheckResourceType)
			}
		}
	}()

	// 1. Create Health Check
	t.Run("CreateHealthCheck", func(t *testing.T) {
		healthCheckName := fmt.Sprintf("formae-test-hc-%s", testID)
		hcProperties := map[string]interface{}{
			"name":               healthCheckName,
			"type":               "HTTP",
			"checkIntervalSec":   10,
			"timeoutSec":         5,
			"healthyThreshold":   2,
			"unhealthyThreshold": 3,
			"httpHealthCheck":    map[string]interface{}{"port": 80, "requestPath": "/"},
		}
		hcPropertiesJSON, _ := json.Marshal(hcProperties)

		createResult, err := healthCheckProvisioner.Create(ctx, &resource.CreateRequest{
			ResourceType: HealthCheckResourceType,
			Properties:   hcPropertiesJSON,
			TargetConfig: testutil.TargetConfig,
		})
		require.NoError(t, err)

		statusResult, err := testutil.WaitForCreate(t, ctx, healthCheckProvisioner, createResult, testutil.TargetConfig, HealthCheckResourceType)
		require.NoError(t, err)
		healthCheckNativeID = statusResult.ProgressResult.NativeID
		t.Logf("Health check created: %s", healthCheckNativeID)
	})

	// 2. Create Backend Service
	t.Run("CreateBackendService", func(t *testing.T) {
		backendServiceName := fmt.Sprintf("formae-test-bs-%s", testID)
		healthCheckSelfLink := fmt.Sprintf("https://www.googleapis.com/compute/v1/%s", healthCheckNativeID)
		bsProperties := map[string]interface{}{
			"name":                backendServiceName,
			"protocol":            "HTTP",
			"timeoutSec":          30,
			"loadBalancingScheme": "EXTERNAL",
			"healthChecks":        []string{healthCheckSelfLink},
		}
		bsPropertiesJSON, _ := json.Marshal(bsProperties)

		createResult, err := backendServiceProvisioner.Create(ctx, &resource.CreateRequest{
			ResourceType: BackendServiceResourceType,
			Properties:   bsPropertiesJSON,
			TargetConfig: testutil.TargetConfig,
		})
		require.NoError(t, err)

		statusResult, err := testutil.WaitForCreate(t, ctx, backendServiceProvisioner, createResult, testutil.TargetConfig, BackendServiceResourceType)
		require.NoError(t, err)
		backendServiceNativeID = statusResult.ProgressResult.NativeID
		t.Logf("Backend service created: %s", backendServiceNativeID)
	})

	// 3. Create URL Map
	t.Run("CreateUrlMap", func(t *testing.T) {
		urlMapName := fmt.Sprintf("formae-test-um-%s", testID)
		backendServiceSelfLink := fmt.Sprintf("https://www.googleapis.com/compute/v1/%s", backendServiceNativeID)
		umProperties := map[string]interface{}{
			"name":           urlMapName,
			"defaultService": backendServiceSelfLink,
		}
		umPropertiesJSON, _ := json.Marshal(umProperties)

		createResult, err := urlMapProvisioner.Create(ctx, &resource.CreateRequest{
			ResourceType: UrlMapResourceType,
			Properties:   umPropertiesJSON,
			TargetConfig: testutil.TargetConfig,
		})
		require.NoError(t, err)

		statusResult, err := testutil.WaitForCreate(t, ctx, urlMapProvisioner, createResult, testutil.TargetConfig, UrlMapResourceType)
		require.NoError(t, err)
		urlMapNativeID = statusResult.ProgressResult.NativeID
		t.Logf("URL map created: %s", urlMapNativeID)
	})

	// 4. Create Target HTTP Proxy
	t.Run("CreateTargetHttpProxy", func(t *testing.T) {
		targetHttpProxyName := fmt.Sprintf("formae-test-thp-%s", testID)
		urlMapSelfLink := fmt.Sprintf("https://www.googleapis.com/compute/v1/%s", urlMapNativeID)
		thpProperties := map[string]interface{}{
			"name":   targetHttpProxyName,
			"urlMap": urlMapSelfLink,
		}
		thpPropertiesJSON, _ := json.Marshal(thpProperties)

		createResult, err := targetHttpProxyProvisioner.Create(ctx, &resource.CreateRequest{
			ResourceType: TargetHttpProxyResourceType,
			Properties:   thpPropertiesJSON,
			TargetConfig: testutil.TargetConfig,
		})
		require.NoError(t, err)

		statusResult, err := testutil.WaitForCreate(t, ctx, targetHttpProxyProvisioner, createResult, testutil.TargetConfig, TargetHttpProxyResourceType)
		require.NoError(t, err)
		targetHttpProxyNativeID = statusResult.ProgressResult.NativeID
		t.Logf("Target HTTP proxy created: %s", targetHttpProxyNativeID)
	})

	// 5. Create Global Forwarding Rule
	t.Run("CreateGlobalForwardingRule", func(t *testing.T) {
		globalForwardingRuleName := fmt.Sprintf("formae-test-gfr-%s", testID)
		targetHttpProxySelfLink := fmt.Sprintf("https://www.googleapis.com/compute/v1/%s", targetHttpProxyNativeID)
		gfrProperties := map[string]interface{}{
			"name":                globalForwardingRuleName,
			"IPProtocol":          "TCP",
			"portRange":           "80",
			"target":              targetHttpProxySelfLink,
			"loadBalancingScheme": "EXTERNAL",
		}
		gfrPropertiesJSON, _ := json.Marshal(gfrProperties)

		createResult, err := globalForwardingRuleProvisioner.Create(ctx, &resource.CreateRequest{
			ResourceType: GlobalForwardingRuleResourceType,
			Properties:   gfrPropertiesJSON,
			TargetConfig: testutil.TargetConfig,
		})
		require.NoError(t, err)

		statusResult, err := testutil.WaitForCreate(t, ctx, globalForwardingRuleProvisioner, createResult, testutil.TargetConfig, GlobalForwardingRuleResourceType)
		require.NoError(t, err)
		globalForwardingRuleNativeID = statusResult.ProgressResult.NativeID
		t.Logf("Global forwarding rule created: %s", globalForwardingRuleNativeID)

		// Verify native ID format
		expectedNativeID := fmt.Sprintf("projects/%s/global/forwardingRules/%s", testutil.Project, globalForwardingRuleName)
		assert.Equal(t, expectedNativeID, globalForwardingRuleNativeID, "Native ID should match expected format")
	})

	// 6. Read and verify Global Forwarding Rule
	t.Run("ReadGlobalForwardingRule", func(t *testing.T) {
		readReq := &resource.ReadRequest{
			NativeID:     globalForwardingRuleNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: GlobalForwardingRuleResourceType,
		}

		readResult, err := globalForwardingRuleProvisioner.Read(ctx, readReq)
		require.NoError(t, err)
		require.Empty(t, readResult.ErrorCode)

		var readProps map[string]interface{}
		err = json.Unmarshal([]byte(readResult.Properties), &readProps)
		require.NoError(t, err)

		assert.NotEmpty(t, readProps["IPAddress"], "Should have an allocated IP address")
		t.Logf("Global forwarding rule IP: %s", readProps["IPAddress"])
	})
}
