// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package container

import (
	"context"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// nodePoolListProvisioner lists node pools by walking the clusters that hold
// them.
//
// A node pool only exists underneath a cluster, and discovery lists with no
// properties at all, so it can name no cluster. GKE has no wildcard in the
// cluster position either - probed live 2026-09-08 against project
// development-477117:
//
//	GET .../locations/us-central1/clusters/-/nodePools
//	  -> 404 "Not found: projects/{p}/locations/us-central1/clusters/-"
//	GET .../locations/-/clusters/-/nodePools
//	  -> 400 "Location \"-\" does not exist"
//
// So unlike Cloud Run's revisions, which do take "services/-", the only way to
// discover a node pool is to walk. Without this the generic builder drops the
// parent collection and asks for /locations/{l}/nodePools, which GKE does not
// have, and the type fails on every discovery cycle.
type nodePoolListProvisioner struct {
	*base.BaseResource
}

func (n *nodePoolListProvisioner) List(
	ctx context.Context,
	request *resource.ListRequest,
) (*resource.ListResult, error) {
	// A caller that names its cluster wants only that one; the generic path
	// builder already handles it.
	if request.AdditionalProperties != nil {
		if parent := request.AdditionalProperties["cluster"]; parent != "" {
			return n.BaseResource.List(ctx, request)
		}
	}

	cfg := config.PathFromTargetConfig(request.TargetConfig)
	if cfg.Project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, n.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	// "locations/-" enumerates every cluster in the project, zonal and
	// regional alike (200, probed live). Walking from there rather than from
	// the target's own location means a zonal cluster is still found when the
	// target names a region, and vice versa.
	clusters, err := n.listClusters(ctx, client, cfg.Project)
	if err != nil {
		return nil, fmt.Errorf("failed to list GKE clusters: %w", err)
	}

	nativeIDs := make([]string, 0, len(clusters))
	for _, cluster := range clusters {
		found, err := n.listNodePools(ctx, client, cluster)
		if err != nil {
			// One unreadable cluster must not hide every other cluster's node
			// pools. A shared project holds clusters the target does not own.
			continue
		}
		nativeIDs = append(nativeIDs, found...)
	}

	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// listClusters returns the resource path of every cluster in the project.
//
// clusters.list is unpaginated - ContainerAPI declares
// Pagination{Disabled: true} because the API accepts no paging parameters at
// all - so there is no token loop here.
func (n *nodePoolListProvisioner) listClusters(
	ctx context.Context, client *transport.Client, project string,
) ([]string, error) {
	url := fmt.Sprintf("%s/projects/%s/locations/-/clusters", n.APIConfig.BaseURL, project)
	response, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: url})
	if err != nil {
		return nil, err
	}

	items, _ := response.Body["clusters"].([]interface{})
	paths := make([]string, 0, len(items))
	for _, raw := range items {
		cluster, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		// GKE reports a cluster's "name" as the bare name and its location
		// separately, so unlike most Google APIs the response carries no full
		// path to reuse. selfLink does, when present.
		if path := pathFromSelfLink(cluster); path != "" {
			paths = append(paths, path)
			continue
		}
		name, _ := cluster["name"].(string)
		location, _ := cluster["location"].(string)
		if name == "" || location == "" {
			continue
		}
		paths = append(paths, fmt.Sprintf("projects/%s/locations/%s/clusters/%s", project, location, name))
	}
	return paths, nil
}

// listNodePools returns the native ID of every node pool under one cluster.
//
// nodePools.list is unpaginated for the same reason clusters.list is.
func (n *nodePoolListProvisioner) listNodePools(
	ctx context.Context, client *transport.Client, clusterPath string,
) ([]string, error) {
	url := fmt.Sprintf("%s/%s/nodePools", n.APIConfig.BaseURL, clusterPath)
	response, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: url})
	if err != nil {
		return nil, err
	}

	items, _ := response.Body["nodePools"].([]interface{})
	out := make([]string, 0, len(items))
	for _, raw := range items {
		pool, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := pool["name"].(string)
		if name == "" {
			continue
		}
		// parseContainerNativeID expects
		// projects/{p}/locations/{l}/clusters/{c}/nodePools/{n}.
		out = append(out, clusterPath+"/nodePools/"+name)
	}
	return out, nil
}

// pathFromSelfLink reduces a cluster's selfLink to the "projects/..." path, or
// returns "" when the response carries no usable one.
func pathFromSelfLink(cluster map[string]interface{}) string {
	selfLink, _ := cluster["selfLink"].(string)
	if i := strings.Index(selfLink, "projects/"); i >= 0 {
		return selfLink[i:]
	}
	return ""
}

// registerNodePoolWalkingList replaces only the List entry; create, read,
// delete and status keep the generic implementation.
//
// It is called from the package init in resources.go rather than from an init
// of its own: Go runs init functions in filename order, and this file sorts
// before "resources.go", so an override registered here would be silently
// replaced by the generic registration.
func registerNodePoolWalkingList() {
	registry.Register(NodePoolResourceType,
		[]resource.Operation{resource.OperationList},
		func(cfg *config.Config) prov.Provisioner {
			def := containerRegistry.Definitions[NodePoolResourceType]
			return &nodePoolListProvisioner{
				BaseResource: &base.BaseResource{
					Config:              cfg,
					APIConfig:           ContainerAPI,
					OperationConfig:     ContainerOperations,
					ResourceConfig:      def.ResourceConfig,
					NativeIDConfig:      ContainerNativeID,
					RequestTransformer:  def.RequestTransformer,
					ResponseTransformer: def.ResponseTransformer,
				},
			}
		})
}
