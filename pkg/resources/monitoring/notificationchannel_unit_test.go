// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package monitoring

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestChannelTypeRoundTrip(t *testing.T) {
	// PKL channelType -> API type on the way in.
	out, err := channelTypeToAPI.Transform(
		map[string]interface{}{"channelType": "email", "displayName": "x"},
		base.TransformContext{})
	if err != nil {
		t.Fatal(err)
	}
	if out["type"] != "email" {
		t.Errorf("type = %v, want email", out["type"])
	}
	if _, ok := out["channelType"]; ok {
		t.Error("channelType should be removed from request body")
	}
	if out["displayName"] != "x" {
		t.Error("other fields must be preserved")
	}

	// API type -> PKL channelType on the way out.
	back := channelTypeFromAPI.Transform(
		map[string]interface{}{"type": "email", "displayName": "x"},
		base.TransformContext{})
	if back["channelType"] != "email" {
		t.Errorf("channelType = %v, want email", back["channelType"])
	}
	if _, ok := back["type"]; ok {
		t.Error("type should be removed from response")
	}
}

func TestMonitoringPathAndNativeID(t *testing.T) {
	ctx := base.PathContext{Project: "p", ResourceType: "notificationChannels"}
	if got := monitoringPathBuilder(ctx); got != "/projects/p/notificationChannels" {
		t.Errorf("collection path = %q", got)
	}
	ctx.ResourceName = "123"
	if got := monitoringPathBuilder(ctx); got != "/projects/p/notificationChannels/123" {
		t.Errorf("resource path = %q", got)
	}
	// Server returns full name; extractor keeps the full path.
	got := extractMonitoringNativeID(
		map[string]interface{}{"name": "projects/p/notificationChannels/123"}, ctx)
	if got != "projects/p/notificationChannels/123" {
		t.Errorf("native id = %q", got)
	}
}

func TestMonitoringResourcesRegistered(t *testing.T) {
	crudl := []resource.Operation{
		resource.OperationCreate, resource.OperationRead,
		resource.OperationUpdate, resource.OperationDelete, resource.OperationList,
	}
	for _, rt := range []string{
		NotificationChannelResourceType, GroupResourceType,
		UptimeCheckConfigResourceType, AlertPolicyResourceType,
	} {
		for _, op := range crudl {
			if !registry.HasProvisioner(rt, op) {
				t.Errorf("%s not registered for %v", rt, op)
			}
		}
	}
}
