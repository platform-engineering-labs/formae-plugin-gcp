// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package servicedirectory

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// Services and endpoints live under a namespace, and discovery lists with no
// properties, so it can name no namespace to look in. Service Directory rejects
// a wildcard for every segment - "namespaces/-" answers "Could not parse
// namespace name", "locations/-" answers "Unsupported location" - so the only
// way to discover either is to walk the collections above them.
type parentWalkListProvisioner struct {
	prov.Provisioner
	cfg *config.Config
	// parentProps are the AdditionalProperties that let the caller name the
	// parents itself; when all are present the generic List already handles it.
	parentProps []string
	// depth is how many collections below the location this resource sits:
	// 1 for services (walk namespaces), 2 for endpoints (walk namespaces, then
	// services).
	depth int
}

// registerParentWalkingLists replaces only the List entry for services and
// endpoints; create, read, update and delete keep the generic implementation.
// It is called from the package init in resources.go rather than from an init
// of its own: Go runs init functions in filename order, and "list.go" sorts
// before "resources.go", so an override registered here would be silently
// replaced by the generic registration.
func registerParentWalkingLists() {
	register := func(resourceType string, parentProps []string, depth int) {
		registry.Register(resourceType,
			[]resource.Operation{resource.OperationList},
			func(cfg *config.Config) prov.Provisioner {
				return &parentWalkListProvisioner{
					Provisioner: serviceDirectoryRegistry.CreateProvisioner(cfg, resourceType),
					cfg:         cfg,
					parentProps: parentProps,
					depth:       depth,
				}
			})
	}
	register(ServiceResourceType, []string{"namespace"}, 1)
	register(EndpointResourceType, []string{"namespace", "service"}, 2)
}

func (p *parentWalkListProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	if p.callerNamedParents(request) {
		return p.Provisioner.List(ctx, request)
	}

	cfg := config.PathFromTargetConfig(request.TargetConfig)
	location := cfg.Location
	if location == "" {
		location = cfg.Region
	}
	if cfg.Project == "" || location == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, p.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	// Walk down from the location, one collection per level, collecting the
	// full paths the API reports. Every level answers with the same
	// {"<collection>": [{"name": "<full path>"}]} shape.
	paths := []string{fmt.Sprintf("%s/projects/%s/locations/%s",
		ServiceDirectoryAPI.BaseURL, cfg.Project, location)}
	collections := []string{"namespaces", "services", "endpoints"}
	for level := 0; level <= p.depth; level++ {
		collection := collections[level]
		next := []string{}
		for _, parent := range paths {
			names, listErr := listNames(ctx, client, parent+"/"+collection, collection)
			if listErr != nil {
				if level == 0 {
					wrapped := transport.WrapError(listErr, "failed to list service directory namespaces")
					return nil, fmt.Errorf("%s", wrapped.Message)
				}
				// One unreadable parent must not hide the rest.
				continue
			}
			for _, name := range names {
				next = append(next, ServiceDirectoryAPI.BaseURL+"/"+name)
			}
		}
		paths = next
	}

	nativeIDs := make([]string, 0, len(paths))
	for _, full := range paths {
		nativeIDs = append(nativeIDs, full[len(ServiceDirectoryAPI.BaseURL)+1:])
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// callerNamedParents reports whether the request names every parent, which is
// the caller telling us exactly where to look.
func (p *parentWalkListProvisioner) callerNamedParents(request *resource.ListRequest) bool {
	if request.AdditionalProperties == nil {
		return false
	}
	for _, prop := range p.parentProps {
		if request.AdditionalProperties[prop] == "" {
			return false
		}
	}
	return true
}

// listNames GETs a collection and returns the full resource path of every item,
// following nextPageToken. Stopping at the first page would silently drop
// namespaces past the API default, and a dropped namespace hides every service
// and endpoint under it.
func listNames(
	ctx context.Context, client *transport.Client, url, collection string,
) ([]string, error) {
	names := []string{}
	pageURL := url
	for {
		resp, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: pageURL})
		if err != nil {
			return nil, err
		}
		items, _ := resp.Body[collection].([]interface{})
		for _, raw := range items {
			item, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if name, _ := item["name"].(string); name != "" {
				names = append(names, name)
			}
		}
		token, _ := resp.Body["nextPageToken"].(string)
		if token == "" {
			return names, nil
		}
		next, qErr := transport.AddQueryParam(url, "pageToken", token)
		if qErr != nil {
			return names, nil
		}
		pageURL = next
	}
}
