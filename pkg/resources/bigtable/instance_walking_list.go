// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigtable

import (
	"context"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// App profiles and the two view types live under an instance, and discovery
// lists with no properties - so it can name no instance to look in, and the
// path builder falls back to a project-level collection URL that addresses
// nothing. The only way to discover any of them is to walk the instances.
type instanceWalkingListProvisioner struct {
	prov.Provisioner
	cfg        *config.Config
	collection string
}

// registerInstanceWalkingLists is called from the package init in resources.go
// so the generic registration is guaranteed to have landed first.
func registerInstanceWalkingLists() {
	for _, spec := range []struct {
		resourceType string
		collection   string
	}{
		{AppProfileResourceType, "appProfiles"},
		{LogicalViewResourceType, "logicalViews"},
		{MaterializedViewResourceType, "materializedViews"},
	} {
		spec := spec
		registry.Register(spec.resourceType,
			[]resource.Operation{resource.OperationList},
			func(cfg *config.Config) prov.Provisioner {
				return &instanceWalkingListProvisioner{
					Provisioner: bigtableRegistry.CreateProvisioner(cfg, spec.resourceType),
					cfg:         cfg,
					collection:  spec.collection,
				}
			})
	}
}

func (p *instanceWalkingListProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	// A named instance is the caller telling us where to look.
	if request.AdditionalProperties != nil && request.AdditionalProperties["instance"] != "" {
		return p.Provisioner.List(ctx, request)
	}

	cfg := config.PathFromTargetConfig(request.TargetConfig)
	if cfg.Project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, p.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	instancesURL := fmt.Sprintf("%s/projects/%s/instances", BigtableAPI.BaseURL, cfg.Project)
	resp, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: instancesURL})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to list bigtable instances")
		return nil, fmt.Errorf("%s", wrapped.Message)
	}

	nativeIDs := []string{}
	instances, _ := resp.Body["instances"].([]interface{})
	for _, raw := range instances {
		instance, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		// An instance reports its full path; the collection URL needs the id.
		instanceName := utils.GetString(instance, "name")
		if instanceName == "" {
			continue
		}
		shortName := instanceName[strings.LastIndex(instanceName, "/")+1:]

		itemsResp, listErr := client.SendRequest(ctx, transport.RequestOptions{
			Method: "GET",
			URL:    fmt.Sprintf("%s/%s/%s", instancesURL, shortName, p.collection),
		})
		if listErr != nil {
			// One unreadable instance must not hide the rest.
			continue
		}
		items, _ := itemsResp.Body[p.collection].([]interface{})
		for _, rawItem := range items {
			item, ok := rawItem.(map[string]interface{})
			if !ok {
				continue
			}
			// Every one of these reports its own full path, which is already
			// the native ID shape.
			if name := utils.GetString(item, "name"); strings.HasPrefix(name, "projects/") {
				nativeIDs = append(nativeIDs, name)
			}
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}
