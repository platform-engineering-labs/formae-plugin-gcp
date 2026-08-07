// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package alloydb

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestPathBuilder(t *testing.T) {
	ctx := base.PathContext{Project: "p", Location: "us-central1", ResourceType: "clusters"}
	if got := alloyDBPathBuilder(ctx); got != "/projects/p/locations/us-central1/clusters" {
		t.Errorf("collection path = %q", got)
	}
	ctx.ResourceName = "cluster1"
	if got := alloyDBPathBuilder(ctx); got != "/projects/p/locations/us-central1/clusters/cluster1" {
		t.Errorf("resource path = %q", got)
	}
}

func TestOperationName(t *testing.T) {
	// A create/delete response is an Operation.
	if got := extractOperationName(map[string]interface{}{
		"name": "projects/p/locations/us-central1/operations/op9",
	}); got != "projects/p/locations/us-central1/operations/op9" {
		t.Errorf("op name = %q", got)
	}
	// A direct resource response is NOT an operation.
	if got := extractOperationName(map[string]interface{}{
		"name": "projects/p/locations/us-central1/clusters/cluster1",
	}); got != "" {
		t.Errorf("resource name should not be treated as op: %q", got)
	}
}

func TestNativeIDFromOperationContext(t *testing.T) {
	// Async create: response is an Operation; native ID built from context.
	ctx := base.PathContext{Project: "p", Location: "us-central1", ResourceType: "clusters", ResourceName: "cluster1"}
	got := extractAlloyDBNativeID(
		map[string]interface{}{"name": "projects/p/locations/us-central1/operations/op9", "done": false}, ctx)
	if got != "projects/p/locations/us-central1/clusters/cluster1" {
		t.Errorf("native id = %q", got)
	}
}

func TestNativeIDFromMetadataTarget(t *testing.T) {
	// No ResourceName in ctx: fall back to the operation's metadata.target.
	ctx := base.PathContext{Project: "p", Location: "us-central1", ResourceType: "clusters"}
	got := extractAlloyDBNativeID(map[string]interface{}{
		"name":     "projects/p/locations/us-central1/operations/op9",
		"metadata": map[string]interface{}{"target": "projects/p/locations/us-central1/clusters/cluster1"},
	}, ctx)
	if got != "projects/p/locations/us-central1/clusters/cluster1" {
		t.Errorf("native id from metadata target = %q", got)
	}
}

func TestOperationStatusChecker(t *testing.T) {
	if done, err := checkOperationStatus(map[string]interface{}{"done": false}); done || err != nil {
		t.Errorf("in-progress: got (%v,%v)", done, err)
	}
	if done, err := checkOperationStatus(map[string]interface{}{"done": true}); !done || err != nil {
		t.Errorf("success: got (%v,%v)", done, err)
	}
	done, err := checkOperationStatus(map[string]interface{}{
		"done": true, "error": map[string]interface{}{"message": "boom"}})
	if !done || err == nil || err.Error() != "boom" {
		t.Errorf("failure: got (%v,%v)", done, err)
	}
}

func TestClusterRegistered(t *testing.T) {
	for _, op := range []resource.Operation{
		resource.OperationCreate, resource.OperationRead,
		resource.OperationDelete, resource.OperationList,
	} {
		if !registry.HasProvisioner(ClusterResourceType, op) {
			t.Errorf("%s not registered for %v", ClusterResourceType, op)
		}
	}
}

func TestNetworkCanonicalizeTransformer(t *testing.T) {
	ctx := base.TransformContext{Project: "development-477117"}
	// GCP canonicalizes the project ID to the project number on read-back;
	// the transformer must rewrite it back to the declared ID.
	got := clusterResponseTransformer.Transform(map[string]interface{}{
		"name": "projects/development-477117/locations/us-central1/clusters/c1",
		"networkConfig": map[string]interface{}{
			"network": "projects/123456789/global/networks/default",
		},
	}, ctx)
	nc := got["networkConfig"].(map[string]interface{})
	if nc["network"] != "projects/development-477117/global/networks/default" {
		t.Errorf("network = %q", nc["network"])
	}
	// ShortNameResponseTransformer still shortens the name.
	if got["name"] != "c1" {
		t.Errorf("name = %q", got["name"])
	}
}

func TestNetworkCanonicalizeNoop(t *testing.T) {
	ctx := base.TransformContext{Project: "p"}
	// No networkConfig: unchanged, no panic.
	got := clusterResponseTransformer.Transform(map[string]interface{}{"name": "c1"}, ctx)
	if got["name"] != "c1" {
		t.Errorf("name = %q", got["name"])
	}
}
