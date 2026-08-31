// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package servicedirectory

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// walkingListProvisioner lists a resource that only exists underneath a
// namespace - or underneath a service inside one - by walking the level above.
//
// Service Directory offers no wildcard anywhere in the hierarchy: locations/-
// answers "Unsupported location: -", and namespaces/- and services/- both
// answer "Could not parse namespace name". Discovery lists with no parent to
// name, so without this a service and an endpoint could never be discovered.
type walkingListProvisioner struct {
	*base.BaseResource
}

func (w *walkingListProvisioner) List(
	ctx context.Context,
	request *resource.ListRequest,
) (*resource.ListResult, error) {
	cfg := config.FromTargetConfig(request.TargetConfig, w.Config.Deps())
	// Everything here is regional, and a location is not something that can be
	// guessed: without one there is no collection to ask about.
	if cfg.Project == "" || cfg.Location == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, w.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	locationURL := fmt.Sprintf("%s/projects/%s/locations/%s", w.APIConfig.BaseURL, cfg.Project, cfg.Location)

	// A caller that names the parents wants only those; the base path builder
	// already composes them.
	if request.AdditionalProperties != nil {
		namespace := request.AdditionalProperties["namespace"]
		service := request.AdditionalProperties["service"]
		if namespace != "" && (w.ResourceConfig.ResourceType == "services" || service != "") {
			return w.BaseResource.List(ctx, request)
		}
	}

	namespaces, err := w.listNames(ctx, client, locationURL+"/namespaces", "namespaces")
	if err != nil {
		return nil, fmt.Errorf("failed to list service directory namespaces: %w", err)
	}

	// Services hang off a namespace; endpoints hang off a service, one level
	// deeper, so the walk collects the services first and then descends.
	parents := namespaces
	if w.ResourceConfig.ResourceType == "endpoints" {
		parents, err = w.descend(ctx, client, namespaces, "services")
		if err != nil {
			return nil, err
		}
	}

	collection := w.ResourceConfig.ResourceType
	nativeIDs := make([]string, 0, len(parents))
	var lastErr error
	failed := 0
	for _, parent := range parents {
		names, err := w.listNames(ctx, client, fmt.Sprintf("%s/%s/%s", w.APIConfig.BaseURL, parent, collection), collection)
		if err != nil {
			// A shared project holds namespaces this target does not own.
			lastErr = err
			failed++
			continue
		}
		nativeIDs = append(nativeIDs, names...)
	}

	// Skipping one unreadable parent is right; skipping every one and reporting
	// an empty list is not - that is indistinguishable from "nothing exists" and
	// impossible to diagnose.
	if len(nativeIDs) == 0 && failed > 0 && failed == len(parents) {
		return nil, fmt.Errorf("could not list %s under any of %d parents: %w", collection, failed, lastErr)
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// descend expands each parent into its children, used to get from namespaces to
// the services that hold the endpoints.
func (w *walkingListProvisioner) descend(
	ctx context.Context, client *transport.Client, parents []string, collection string,
) ([]string, error) {
	var out []string
	for _, parent := range parents {
		names, err := w.listNames(ctx, client,
			fmt.Sprintf("%s/%s/%s", w.APIConfig.BaseURL, parent, collection), collection)
		if err != nil {
			// One unreadable namespace must not hide every other one's services.
			continue
		}
		out = append(out, names...)
	}
	return out, nil
}

// listNames GETs a collection and returns each item's name, which the API
// already reports as the full resource path that serves as the native ID.
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
		if name, ok := item["name"].(string); ok && name != "" {
			out = append(out, name)
		}
	}
	return out, nil
}
