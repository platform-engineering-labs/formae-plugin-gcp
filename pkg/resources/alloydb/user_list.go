// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package alloydb

import (
	"context"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// userListProvisioner overrides List for AlloyDB users and delegates everything
// else to the generic provisioner.
//
// A user lives under a cluster, and discovery lists with no properties, so it
// can name no cluster to look in. Unlike instances.list, users.list rejects the
// "-" wildcard ("Resource 'clusters/-' was not found"), so the only way to
// discover a user is to walk the clusters first.
type userListProvisioner struct {
	prov.Provisioner
	cfg *config.Config
}

// registerUserListOverride replaces only the List entry; create, read and delete
// keep the generic implementation. Called from the package init rather than an
// init() of its own so it cannot depend on filename ordering.
func registerUserListOverride() {
	registry.Register(UserResourceType,
		[]resource.Operation{resource.OperationList},
		func(cfg *config.Config) prov.Provisioner {
			return &userListProvisioner{
				Provisioner: alloyDBRegistry.CreateProvisioner(cfg, UserResourceType),
				cfg:         cfg,
			}
		})
}

func (p *userListProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	// A named cluster is the caller telling us where to look; the generic
	// implementation already handles that.
	if request.AdditionalProperties != nil && request.AdditionalProperties["cluster"] != "" {
		return p.Provisioner.List(ctx, request)
	}

	cfg := config.FromTargetConfig(request.TargetConfig, nil /* path context only; this config never authenticates */)
	project, location := "", ""
	if cfg != nil {
		project = cfg.Project
		location = cfg.Location
		if location == "" {
			location = cfg.Region
		}
	}
	if project == "" || location == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, p.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}
	base := fmt.Sprintf("%s/projects/%s/locations/%s/clusters",
		AlloyDBAPI.BaseURL, project, location)

	resp, rErr := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: base})
	if rErr != nil {
		wrapped := transport.WrapError(rErr, "failed to list alloydb clusters")
		return nil, fmt.Errorf("%s", wrapped.Message)
	}

	nativeIDs := []string{}
	clusters, _ := resp.Body["clusters"].([]interface{})
	for _, raw := range clusters {
		cluster, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		// A cluster reports its full path, which is the users collection's parent.
		fullName, _ := cluster["name"].(string)
		if !strings.Contains(fullName, "/clusters/") {
			continue
		}
		usersResp, listErr := client.SendRequest(ctx, transport.RequestOptions{
			Method: "GET",
			URL:    fmt.Sprintf("%s/%s/users", AlloyDBAPI.BaseURL, fullName),
		})
		if listErr != nil {
			// One unreadable cluster must not hide the rest.
			continue
		}
		users, _ := usersResp.Body["users"].([]interface{})
		for _, rawUser := range users {
			user, ok := rawUser.(map[string]interface{})
			if !ok {
				continue
			}
			// A user reports its full path, which is already the native ID shape.
			if name, _ := user["name"].(string); strings.Contains(name, "/users/") {
				nativeIDs = append(nativeIDs, name)
			}
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}
