// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package container

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// GKE accepts "-" in the location position, probed live 2026-09-08 against
// project development-477117: GET /projects/{p}/locations/-/clusters -> 200.
// A target that sets only region or only zone would otherwise interpolate the
// empty string and ask for "locations//clusters".
//
// A location may legitimately be a zone here (a zonal cluster) or a region (a
// regional one), so there is nothing to fall back to the way Cloud Run falls
// back to region - the wildcard is the only answer that covers both.
func TestClusterListWildcardsEmptyLocation(t *testing.T) {
	got := containerPathBuilder(base.PathContext{
		Project: "p", ResourceType: "clusters", IsList: true,
	})
	if want := "/projects/p/locations/-/clusters"; got != want {
		t.Errorf("parentless cluster list = %q, want %q", got, want)
	}
}

// The wildcard is for the empty case only: a target that names a location gets
// that location, and a read/create/delete must never address "-".
func TestClusterPathsKeepAConcreteLocation(t *testing.T) {
	listCtx := base.PathContext{
		Project: "p", Location: "us-central1", ResourceType: "clusters", IsList: true,
	}
	if got, want := containerPathBuilder(listCtx), "/projects/p/locations/us-central1/clusters"; got != want {
		t.Errorf("located list = %q, want %q", got, want)
	}

	readCtx := base.PathContext{
		Project: "p", ResourceType: "clusters", ResourceName: "c1",
	}
	if got, want := containerPathBuilder(readCtx), "/projects/p/locations//clusters/c1"; got != want {
		// A read with no location is a caller error, not something to paper
		// over with a wildcard that would address every cluster at once.
		t.Errorf("unlocated read = %q, want %q", got, want)
	}
}

// GKE has no wildcard in the cluster position - probed live: GET
// .../locations/us-central1/clusters/-/nodePools answers 404 "Not found:
// projects/{p}/locations/us-central1/clusters/-", and with locations/- as well
// it answers 400 "Location \"-\" does not exist". So a node pool can only be
// discovered by walking the clusters, and List must be overridden.
func TestNodePoolListIsOverridden(t *testing.T) {
	if !registry.HasProvisioner(NodePoolResourceType, resource.OperationList) {
		t.Fatalf("%s not registered for List", NodePoolResourceType)
	}
	provisioner := registry.Get(NodePoolResourceType, resource.OperationList, &config.Config{})
	if _, ok := provisioner.(*nodePoolListProvisioner); !ok {
		t.Errorf("List provisioner is %T, want *nodePoolListProvisioner - the generic "+
			"one addresses /locations/{l}/nodePools, which GKE does not have", provisioner)
	}
}

// Every other operation must keep the generic implementation; only List differs.
func TestNodePoolKeepsGenericLifecycle(t *testing.T) {
	for _, op := range []resource.Operation{
		resource.OperationCreate, resource.OperationRead,
		resource.OperationDelete, resource.OperationCheckStatus,
	} {
		provisioner := registry.Get(NodePoolResourceType, op, &config.Config{})
		if _, ok := provisioner.(*nodePoolListProvisioner); ok {
			t.Errorf("%v wrongly uses the List walker", op)
		}
	}
}

// A zonal cluster reports its path with "zones", not "locations". Rejecting it
// left clusterName unset, and discovery then never listed that cluster's node
// pools - the shape seen in production as "Missing parent property
// property=clusterName parent_id=projects/p/zones/europe-west4-a/clusters/c".
func TestParseClusterPathAcceptsZonalPaths(t *testing.T) {
	for _, id := range []string{
		"projects/development-477117/zones/europe-west4-a/clusters/connect-matrix",
		"projects/development-477117/locations/europe-west4/clusters/connect-matrix",
	} {
		project, location, name, err := ParseClusterPath(id)
		if err != nil {
			t.Fatalf("ParseClusterPath(%q) = %v", id, err)
		}
		if project != "development-477117" || name != "connect-matrix" || location == "" {
			t.Fatalf("ParseClusterPath(%q) = %q/%q/%q", id, project, location, name)
		}
	}
	if _, _, _, err := ParseClusterPath("projects/p/regions/r/clusters/c"); err == nil {
		t.Fatal("ParseClusterPath accepted a path that is not a GKE cluster")
	}
}
