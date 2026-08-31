// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package spanner

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// Databases and backup schedules live under an instance, and discovery lists
// with no properties, so it can name no instance to look in. Spanner rejects a
// wildcard for both - "instances/-/databases" and "databases/-/backupSchedules"
// answer 400 "Invalid List... request" - so the only way to discover either is
// to walk the collections above them. ("instances/-/instancePartitions" is
// accepted, so the rejection is per-collection rather than a rule.)
type parentWalkListProvisioner struct {
	prov.Provisioner
	cfg *config.Config
	// parentProps are the AdditionalProperties that let the caller name the
	// parents itself; when all are present the generic List already handles it.
	parentProps []string
	// depth is how many collections below the project this resource sits:
	// 1 for databases (walk instances), 2 for backup schedules (walk instances,
	// then databases).
	depth int
}

// registerParentWalkingLists replaces only the List entry for databases and
// backup schedules; create, read, update and delete keep the generic
// implementation. It is called from the package init in resources.go rather
// than from an init of its own: Go runs init functions in filename order, and
// "list.go" sorts before "resources.go", so an override registered here would
// be silently replaced by the generic registration.
func registerParentWalkingLists() {
	register := func(resourceType string, parentProps []string, depth int) {
		registry.Register(resourceType,
			[]resource.Operation{resource.OperationList},
			func(cfg *config.Config) prov.Provisioner {
				return &parentWalkListProvisioner{
					Provisioner: spannerRegistry.CreateProvisioner(cfg, resourceType),
					cfg:         cfg,
					parentProps: parentProps,
					depth:       depth,
				}
			})
	}
	register(DatabaseResourceType, []string{"instance"}, 1)
	register(BackupScheduleResourceType, []string{"instance", "database"}, 2)
}

func (p *parentWalkListProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	if p.callerNamedParents(request) {
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

	// Walk down from the project, one collection per level, collecting the full
	// paths the API reports. Every level answers with the same
	// {"<collection>": [{"name": "<full path>"}]} shape.
	paths := []string{fmt.Sprintf("%s/projects/%s", SpannerAPI.BaseURL, cfg.Project)}
	collections := []string{"instances", "databases", "backupSchedules"}
	for level := 0; level <= p.depth; level++ {
		collection := collections[level]
		next := []string{}
		for _, parent := range paths {
			names, listErr := listNames(ctx, client, parent+"/"+collection, collection)
			if listErr != nil {
				if level == 0 {
					wrapped := transport.WrapError(listErr, "failed to list spanner instances")
					return nil, fmt.Errorf("%s", wrapped.Message)
				}
				// One unreadable parent must not hide the rest.
				continue
			}
			for _, name := range names {
				next = append(next, SpannerAPI.BaseURL+"/"+name)
			}
		}
		paths = next
	}

	nativeIDs := make([]string, 0, len(paths))
	for _, full := range paths {
		nativeIDs = append(nativeIDs, full[len(SpannerAPI.BaseURL)+1:])
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
// instances past the API default, and a dropped instance hides every database
// and backup schedule under it.
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
