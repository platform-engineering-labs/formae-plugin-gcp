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
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/testutil"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// natTestScaffold holds a Network + Router ready for RouterNat tests. Callers
// must defer cleanup.
type natTestScaffold struct {
	ctx             context.Context
	network         prov.Provisioner
	router          prov.Provisioner
	routerNat       prov.Provisioner
	networkNativeID string
	networkSelfLink string
	routerNativeID  string
	routerName      string
	cleanup         func(*testing.T)
}

func setupRouterNatScaffold(t *testing.T, suffix string) *natTestScaffold {
	t.Helper()

	network, err := NewComputeProvisioner(testutil.Config, NetworkResourceType)
	require.NoError(t, err, "Failed to create network provisioner")
	router, err := NewComputeProvisioner(testutil.Config, RouterResourceType)
	require.NoError(t, err, "Failed to create router provisioner")
	routerNat, err := NewComputeProvisioner(testutil.Config, RouterNatResourceType)
	require.NoError(t, err, "Failed to create router-nat provisioner — is RouterNatResourceType registered?")

	netName := fmt.Sprintf("formae-test-nat-net-%s-%s", suffix, uuid.New().String()[:8])
	routerName := fmt.Sprintf("formae-test-nat-rtr-%s-%s", suffix, uuid.New().String()[:8])
	ctx := context.Background()

	// Create network
	netPropsJSON, _ := json.Marshal(map[string]interface{}{
		"name":                  netName,
		"autoCreateSubnetworks": false,
	})
	netCreate, err := network.Create(ctx, &resource.CreateRequest{
		ResourceType: NetworkResourceType,
		Properties:   netPropsJSON,
		TargetConfig: testutil.TargetConfig,
	})
	require.NoError(t, err)
	netStatus, err := testutil.WaitForCreate(t, ctx, network, netCreate, testutil.TargetConfig, NetworkResourceType)
	require.NoError(t, err, "network create must succeed")

	netNativeID := netStatus.ProgressResult.NativeID
	netSelfLink := utils.GetString(utils.MustParseProperties(netStatus.ProgressResult.ResourceProperties), "selfLink")
	require.NotEmpty(t, netSelfLink)

	// Create router
	routerPropsJSON, _ := json.Marshal(map[string]interface{}{
		"name":    routerName,
		"network": netSelfLink,
	})
	rtrCreate, err := router.Create(ctx, &resource.CreateRequest{
		ResourceType: RouterResourceType,
		Properties:   routerPropsJSON,
		TargetConfig: testutil.TargetConfig,
	})
	require.NoError(t, err)
	rtrStatus, err := testutil.WaitForCreate(t, ctx, router, rtrCreate, testutil.TargetConfig, RouterResourceType)
	require.NoError(t, err, "router create must succeed")
	rtrNativeID := rtrStatus.ProgressResult.NativeID

	s := &natTestScaffold{
		ctx:             ctx,
		network:         network,
		router:          router,
		routerNat:       routerNat,
		networkNativeID: netNativeID,
		networkSelfLink: netSelfLink,
		routerNativeID:  rtrNativeID,
		routerName:      routerName,
	}

	s.cleanup = func(t *testing.T) {
		// Best-effort: delete router then network. RouterNat deletion is the
		// caller's responsibility (so individual tests can verify their own
		// cleanup).
		delResult, err := router.Delete(ctx, &resource.DeleteRequest{
			NativeID:     rtrNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: RouterResourceType,
		})
		if err == nil && delResult != nil && delResult.ProgressResult != nil {
			_, _ = testutil.WaitForDelete(t, ctx, router, delResult, testutil.TargetConfig, RouterResourceType)
		}

		delResult2, err := network.Delete(ctx, &resource.DeleteRequest{
			NativeID:     netNativeID,
			TargetConfig: testutil.TargetConfig,
			ResourceType: NetworkResourceType,
		})
		if err == nil && delResult2 != nil && delResult2.ProgressResult != nil {
			_, _ = testutil.WaitForDelete(t, ctx, network, delResult2, testutil.TargetConfig, NetworkResourceType)
		}
	}
	return s
}

// TestRouterNatCreate exercises the full Create→Read→List→Delete cycle.
func TestRouterNatCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	s := setupRouterNatScaffold(t, "create")
	defer s.cleanup(t)

	natName := fmt.Sprintf("nat-%s", uuid.New().String()[:8])
	t.Logf("Creating RouterNat %s on router %s", natName, s.routerName)

	natPropsJSON, _ := json.Marshal(map[string]interface{}{
		"name":                          natName,
		"router":                        s.routerName,
		"region":                        testutil.Region,
		"natIpAllocateOption":           "AUTO_ONLY",
		"sourceSubnetworkIpRangesToNat": "ALL_SUBNETWORKS_ALL_IP_RANGES",
	})

	natProv := s.routerNat
	createResult, err := natProv.Create(s.ctx, &resource.CreateRequest{
		ResourceType: RouterNatResourceType,
		Properties:   natPropsJSON,
		TargetConfig: testutil.TargetConfig,
	})
	require.NoError(t, err)
	require.NotNil(t, createResult)
	require.NotNil(t, createResult.ProgressResult)
	if createResult.ProgressResult.OperationStatus == resource.OperationStatusFailure {
		t.Fatalf("RouterNat creation failed: %s (code=%s)",
			createResult.ProgressResult.StatusMessage, createResult.ProgressResult.ErrorCode)
	}

	statusResult, err := testutil.WaitForCreate(t, s.ctx, s.routerNat, createResult, testutil.TargetConfig, RouterNatResourceType)
	require.NoError(t, err, "RouterNat create must complete")

	natNativeID := statusResult.ProgressResult.NativeID
	require.NotEmpty(t, natNativeID)
	expectedNativeIDPrefix := fmt.Sprintf("projects/%s/regions/%s/routers/%s/nats/%s",
		testutil.Project, testutil.Region, s.routerName, natName)
	assert.Equal(t, expectedNativeIDPrefix, natNativeID, "NativeID should be the synthetic NAT path")

	t.Run("Read", func(t *testing.T) {
		readResult, err := natProv.Read(s.ctx, &resource.ReadRequest{
			NativeID:     natNativeID,
			ResourceType: RouterNatResourceType,
			TargetConfig: testutil.TargetConfig,
		})
		require.NoError(t, err)
		require.Empty(t, readResult.ErrorCode)
		require.NotEmpty(t, readResult.Properties)

		readProps := utils.MustParseProperties(readResult.Properties)
		assert.Equal(t, natName, readProps["name"])
		assert.Equal(t, "AUTO_ONLY", readProps["natIpAllocateOption"])
		assert.Equal(t, "ALL_SUBNETWORKS_ALL_IP_RANGES", readProps["sourceSubnetworkIpRangesToNat"])
	})

	t.Run("List", func(t *testing.T) {
		listResult, err := natProv.List(s.ctx, &resource.ListRequest{
			ResourceType: RouterNatResourceType,
			TargetConfig: testutil.TargetConfig,
		})
		require.NoError(t, err)
		require.NotNil(t, listResult.NativeIDs)
		found := false
		for _, id := range listResult.NativeIDs {
			if id == natNativeID {
				found = true
				break
			}
		}
		assert.True(t, found, "List should include the created RouterNat")
	})

	t.Run("Delete", func(t *testing.T) {
		deleteResult, err := natProv.Delete(s.ctx, &resource.DeleteRequest{
			NativeID:     natNativeID,
			ResourceType: RouterNatResourceType,
			TargetConfig: testutil.TargetConfig,
		})
		require.NoError(t, err)
		_, err = testutil.WaitForDelete(t, s.ctx, s.routerNat, deleteResult, testutil.TargetConfig, RouterNatResourceType)
		require.NoError(t, err)
	})

	t.Run("VerifyDeleted", func(t *testing.T) {
		readResult, err := natProv.Read(s.ctx, &resource.ReadRequest{
			NativeID:     natNativeID,
			ResourceType: RouterNatResourceType,
			TargetConfig: testutil.TargetConfig,
		})
		require.NoError(t, err)
		assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode)
	})
}

// TestRouterNatRejectedSecondPreservesFirst checks read-modify-write integrity:
// when GCP rejects a second NAT, the first must still exist. We use the
// constraint that only one NAT per router can use ALL_SUBNETWORKS_ALL_IP_RANGES.
func TestRouterNatRejectedSecondPreservesFirst(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	s := setupRouterNatScaffold(t, "rejected")
	defer s.cleanup(t)

	natA := fmt.Sprintf("nat-a-%s", uuid.New().String()[:8])
	natB := fmt.Sprintf("nat-b-%s", uuid.New().String()[:8])
	natProv := s.routerNat

	// Create natA — succeeds.
	aPropsJSON, _ := json.Marshal(map[string]interface{}{
		"name":                          natA,
		"router":                        s.routerName,
		"region":                        testutil.Region,
		"natIpAllocateOption":           "AUTO_ONLY",
		"sourceSubnetworkIpRangesToNat": "ALL_SUBNETWORKS_ALL_IP_RANGES",
	})
	aCreate, err := natProv.Create(s.ctx, &resource.CreateRequest{
		ResourceType: RouterNatResourceType,
		Properties:   aPropsJSON,
		TargetConfig: testutil.TargetConfig,
	})
	require.NoError(t, err)
	if aCreate.ProgressResult.OperationStatus == resource.OperationStatusFailure {
		t.Fatalf("natA create failed: %s (code=%s)",
			aCreate.ProgressResult.StatusMessage, aCreate.ProgressResult.ErrorCode)
	}
	aStatus, err := testutil.WaitForCreate(t, s.ctx, s.routerNat, aCreate, testutil.TargetConfig, RouterNatResourceType)
	require.NoError(t, err)
	natAID := aStatus.ProgressResult.NativeID
	t.Logf("natA created: %s", natAID)

	// Try to create natB with the same conflicting option — GCP must reject.
	bPropsJSON, _ := json.Marshal(map[string]interface{}{
		"name":                          natB,
		"router":                        s.routerName,
		"region":                        testutil.Region,
		"natIpAllocateOption":           "AUTO_ONLY",
		"sourceSubnetworkIpRangesToNat": "ALL_SUBNETWORKS_ALL_IP_RANGES",
	})
	bCreate, err := natProv.Create(s.ctx, &resource.CreateRequest{
		ResourceType: RouterNatResourceType,
		Properties:   bPropsJSON,
		TargetConfig: testutil.TargetConfig,
	})
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusFailure, bCreate.ProgressResult.OperationStatus,
		"natB Create should fail — GCP allows only one ALL_SUBNETWORKS_ALL_IP_RANGES NAT per router")
	t.Logf("natB rejected as expected: %s (code=%s)",
		bCreate.ProgressResult.StatusMessage, bCreate.ProgressResult.ErrorCode)

	// natA must still exist after the rejection.
	readA, err := natProv.Read(s.ctx, &resource.ReadRequest{
		NativeID:     natAID,
		ResourceType: RouterNatResourceType,
		TargetConfig: testutil.TargetConfig,
	})
	require.NoError(t, err)
	require.Empty(t, readA.ErrorCode, "natA must still exist — failed second create must not trample it")

	props := utils.MustParseProperties(readA.Properties)
	assert.Equal(t, natA, props["name"])
	assert.Equal(t, "AUTO_ONLY", props["natIpAllocateOption"])

	// Cleanup natA.
	delA, err := natProv.Delete(s.ctx, &resource.DeleteRequest{
		NativeID:     natAID,
		ResourceType: RouterNatResourceType,
		TargetConfig: testutil.TargetConfig,
	})
	require.NoError(t, err)
	_, _ = testutil.WaitForDelete(t, s.ctx, s.routerNat, delA, testutil.TargetConfig, RouterNatResourceType)
}

// TestRouterNatUpdate verifies that flipping logConfig.enable persists.
func TestRouterNatUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	s := setupRouterNatScaffold(t, "update")
	defer s.cleanup(t)

	natName := fmt.Sprintf("nat-upd-%s", uuid.New().String()[:8])
	natProv := s.routerNat
	createJSON, _ := json.Marshal(map[string]interface{}{
		"name":                          natName,
		"router":                        s.routerName,
		"region":                        testutil.Region,
		"natIpAllocateOption":           "AUTO_ONLY",
		"sourceSubnetworkIpRangesToNat": "ALL_SUBNETWORKS_ALL_IP_RANGES",
		"logConfig": map[string]interface{}{
			"enable": false,
			"filter": "ERRORS_ONLY",
		},
	})
	createResult, err := natProv.Create(s.ctx, &resource.CreateRequest{
		ResourceType: RouterNatResourceType,
		Properties:   createJSON,
		TargetConfig: testutil.TargetConfig,
	})
	require.NoError(t, err)
	statusResult, err := testutil.WaitForCreate(t, s.ctx, s.routerNat, createResult, testutil.TargetConfig, RouterNatResourceType)
	require.NoError(t, err)
	natNativeID := statusResult.ProgressResult.NativeID

	// Update: flip enable to true.
	updJSON, _ := json.Marshal(map[string]interface{}{
		"name":                          natName,
		"router":                        s.routerName,
		"region":                        testutil.Region,
		"natIpAllocateOption":           "AUTO_ONLY",
		"sourceSubnetworkIpRangesToNat": "ALL_SUBNETWORKS_ALL_IP_RANGES",
		"logConfig": map[string]interface{}{
			"enable": true,
			"filter": "ALL",
		},
	})

	updateResult, err := natProv.Update(s.ctx, &resource.UpdateRequest{
		ResourceType:      RouterNatResourceType,
		DesiredProperties: updJSON,
		NativeID:          natNativeID,
		TargetConfig:      testutil.TargetConfig,
	})
	require.NoError(t, err)
	if updateResult.ProgressResult.OperationStatus == resource.OperationStatusInProgress {
		_, err = testutil.WaitForStatus(t, s.ctx, s.routerNat, &resource.StatusRequest{
			RequestID:    updateResult.ProgressResult.RequestID,
			ResourceType: RouterNatResourceType,
			TargetConfig: testutil.TargetConfig,
		})
		require.NoError(t, err)
	}

	readResult, err := natProv.Read(s.ctx, &resource.ReadRequest{
		NativeID:     natNativeID,
		ResourceType: RouterNatResourceType,
		TargetConfig: testutil.TargetConfig,
	})
	require.NoError(t, err)
	props := utils.MustParseProperties(readResult.Properties)
	logCfg, _ := props["logConfig"].(map[string]interface{})
	require.NotNil(t, logCfg, "logConfig must survive update")
	assert.Equal(t, true, logCfg["enable"])
	assert.Equal(t, "ALL", logCfg["filter"])

	// Cleanup
	delResult, _ := natProv.Delete(s.ctx, &resource.DeleteRequest{
		NativeID:     natNativeID,
		ResourceType: RouterNatResourceType,
		TargetConfig: testutil.TargetConfig,
	})
	_, _ = testutil.WaitForDelete(t, s.ctx, s.routerNat, delResult, testutil.TargetConfig, RouterNatResourceType)
}

// TestRouterNatNotFound: Read against a missing NAT returns NotFound.
func TestRouterNatNotFound(t *testing.T) {
	prov, err := NewComputeProvisioner(testutil.Config, RouterNatResourceType)
	require.NoError(t, err)

	readReq := &resource.ReadRequest{
		NativeID: fmt.Sprintf("projects/%s/regions/%s/routers/nonexistent-router/nats/nonexistent-nat",
			testutil.Project, testutil.Region),
		TargetConfig: testutil.TargetConfig,
		ResourceType: RouterNatResourceType,
	}

	readResult, err := prov.Read(context.Background(), readReq)
	require.NoError(t, err)
	require.NotNil(t, readResult)
	assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode)
}
