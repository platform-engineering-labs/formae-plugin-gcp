// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package sql

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// databaseProvisioner lists databases by walking the instances that hold them.
//
// A database only exists underneath an instance, and Cloud SQL offers no way to
// ask across instances: /projects/{p}/databases is a 404 and
// /projects/{p}/instances/-/databases is a 400, so there is no wildcard to
// substitute the way privateca and Datastream have one. Discovery lists with no
// parent to name, so without this a database could never be discovered.
type databaseProvisioner struct {
	*base.BaseResource
}

func (d *databaseProvisioner) List(
	ctx context.Context,
	request *resource.ListRequest,
) (*resource.ListResult, error) {
	// A caller that names its instance wants only that one; the base path
	// builder already handles it.
	if request.AdditionalProperties != nil {
		if parent := request.AdditionalProperties["instance"]; parent != "" {
			return d.BaseResource.List(ctx, request)
		}
	}

	cfg := config.FromTargetConfig(request.TargetConfig, d.Config.Deps())
	if cfg.Project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, d.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	instances, err := d.listNames(ctx, client,
		fmt.Sprintf("%s/projects/%s/instances", d.APIConfig.BaseURL, cfg.Project))
	if err != nil {
		return nil, fmt.Errorf("failed to list Cloud SQL instances: %w", err)
	}

	nativeIDs := make([]string, 0, len(instances))
	var lastErr error
	failed := 0
	for _, instance := range instances {
		names, err := d.listNames(ctx, client, fmt.Sprintf("%s/projects/%s/instances/%s/databases",
			d.APIConfig.BaseURL, cfg.Project, instance))
		if err != nil {
			// A shared project holds instances this target does not own, and an
			// instance still creating refuses the call.
			lastErr = err
			failed++
			continue
		}
		for _, name := range names {
			nativeIDs = append(nativeIDs, fmt.Sprintf("projects/%s/instances/%s/databases/%s",
				cfg.Project, instance, name))
		}
	}

	// Skipping one unreadable instance is right; skipping every one and
	// reporting an empty list is not - that is indistinguishable from "no
	// databases exist" and impossible to diagnose.
	if len(nativeIDs) == 0 && failed > 0 && failed == len(instances) {
		return nil, fmt.Errorf("could not list databases on any of %d instances: %w", failed, lastErr)
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// listNames GETs a Cloud SQL collection and returns each item's name.
func (d *databaseProvisioner) listNames(
	ctx context.Context, client *transport.Client, url string,
) ([]string, error) {
	response, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: url})
	if err != nil {
		return nil, err
	}
	items, _ := response.Body["items"].([]interface{})
	out := make([]string, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if name, ok := item["name"].(string); ok && name != "" {
			out = append(out, name)
		}
	}
	return out, nil
}
