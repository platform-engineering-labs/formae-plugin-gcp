// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package filestore

import (
	"context"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// snapshotProvisioner lists snapshots by walking the instances that hold them.
//
// A snapshot only exists underneath an instance, and Filestore has no wildcard
// in the instance position - unlike privateca, where "caPools/-" works.
// Discovery lists with no parent to name, so without this it would ask for a
// URL with an empty segment and find nothing.
type snapshotProvisioner struct {
	*base.BaseResource
}

func (s *snapshotProvisioner) List(
	ctx context.Context,
	request *resource.ListRequest,
) (*resource.ListResult, error) {
	// A caller that names its instance wants only that one; the base path
	// builder already handles it.
	if request.AdditionalProperties != nil {
		if parent := request.AdditionalProperties["instance"]; parent != "" {
			return s.BaseResource.List(ctx, request)
		}
	}

	cfg := config.FromTargetConfig(request.TargetConfig, s.Config.Deps())
	if cfg.Location == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, s.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	instances, err := collectNames(ctx, client,
		fmt.Sprintf("%s/projects/%s/locations/%s/instances", s.APIConfig.BaseURL, cfg.Project, cfg.Location),
		"instances")
	if err != nil {
		return nil, fmt.Errorf("failed to list Filestore instances: %w", err)
	}

	nativeIDs := make([]string, 0, len(instances))
	for _, instance := range instances {
		// "instance" is already a full resource path.
		found, err := collectNames(ctx, client,
			s.APIConfig.BaseURL+"/"+instance+"/snapshots", "snapshots")
		if err != nil {
			// One unreadable instance must not hide every other instance's
			// snapshots. Skip it and keep walking.
			continue
		}
		nativeIDs = append(nativeIDs, found...)
	}

	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// collectNames GETs one collection URL and returns the full resource path of
// every item, following nextPageToken to the end.
func collectNames(ctx context.Context, client *transport.Client, url, itemsKey string) ([]string, error) {
	var out []string
	next := url
	for next != "" {
		response, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: next})
		if err != nil {
			return nil, err
		}
		items, _ := response.Body[itemsKey].([]interface{})
		for _, raw := range items {
			item, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			name, ok := item["name"].(string)
			if !ok || name == "" {
				continue
			}
			if i := strings.Index(name, "projects/"); i >= 0 {
				name = name[i:]
			}
			out = append(out, name)
		}
		token, _ := response.Body["nextPageToken"].(string)
		if token == "" {
			break
		}
		next, err = transport.AddQueryParam(url, "pageToken", token)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
