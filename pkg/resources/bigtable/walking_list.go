// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigtable

import (
	"context"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// walkingListProvisioner lists a resource that only exists underneath an
// instance by walking the instances first.
//
// Discovery lists with no parent to name, and base leaves the parent empty so a
// path builder can substitute the API's wildcard. Bigtable has none at the
// instance level: /projects/{p}/backups and /projects/{p}/materializedViews are
// both unmatched routes. Walking the instances is the only way either type is
// ever discovered. Backups sit one level deeper still, under a cluster, and
// there the API does offer a wildcard - clusters/- lists an instance's backups
// across all of its clusters.
type walkingListProvisioner struct {
	*BigtableProvisioner
}

func (w *walkingListProvisioner) List(
	ctx context.Context,
	request *resource.ListRequest,
) (*resource.ListResult, error) {
	// A caller that names its instance wants only that one; the base path
	// builder already handles it.
	if request.AdditionalProperties != nil {
		if parent := request.AdditionalProperties["instance"]; parent != "" {
			return w.BigtableProvisioner.List(ctx, request)
		}
	}

	cfg := config.FromTargetConfig(request.TargetConfig, w.Config.Deps())
	if cfg.Project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, w.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	instances, err := w.listNames(ctx, client,
		fmt.Sprintf("%s/projects/%s/instances", w.APIConfig.BaseURL, cfg.Project), "instances")
	if err != nil {
		return nil, fmt.Errorf("failed to list Bigtable instances: %w", err)
	}

	// A backup is addressed through its cluster, so the collection needs the
	// cluster wildcard; a materialized view hangs off the instance directly.
	collection := w.ResourceConfig.ResourceType
	suffix := collection
	if collection == "backups" {
		suffix = "clusters/-/backups"
	}

	nativeIDs := make([]string, 0, len(instances))
	var lastErr error
	failed := 0
	for _, instance := range instances {
		names, err := w.listNames(ctx, client,
			fmt.Sprintf("%s/%s/%s", w.APIConfig.BaseURL, instance, suffix), collection)
		if err != nil {
			// A shared project holds instances this target does not own, and an
			// instance still creating refuses the call.
			lastErr = err
			failed++
			continue
		}
		nativeIDs = append(nativeIDs, names...)
	}

	// Skipping one unreadable instance is right; skipping every one and
	// reporting an empty list is not - that is indistinguishable from "nothing
	// exists" and impossible to diagnose.
	if len(nativeIDs) == 0 && failed > 0 && failed == len(instances) {
		return nil, fmt.Errorf("could not list %s on any of %d instances: %w", collection, failed, lastErr)
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// listNames GETs a Bigtable collection and returns each item's name, which the
// API already reports as the full resource path that serves as the native ID.
func (w *walkingListProvisioner) listNames(
	ctx context.Context, client *transport.Client, url, itemsKey string,
) ([]string, error) {
	response, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: url})
	if err != nil {
		return nil, err
	}
	items, _ := response.Body[itemsKey].([]interface{})
	out := make([]string, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := item["name"].(string)
		if name == "" {
			continue
		}
		out = append(out, strings.TrimPrefix(name, "/"))
	}
	return out, nil
}
