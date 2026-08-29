// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package apigateway

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// configWalkingProvisioner lists api configs by walking the apis that hold them.
//
// A config is addressed underneath its api and there is no wildcard in the api
// position, while discovery lists with no parent to name - so without this a
// config could never be discovered.
type configWalkingProvisioner struct {
	*base.BaseResource
}

func (c *configWalkingProvisioner) List(
	ctx context.Context,
	request *resource.ListRequest,
) (*resource.ListResult, error) {
	// A caller that names its api wants only that one; the base path builder
	// already handles it.
	if request.AdditionalProperties != nil {
		if parent := request.AdditionalProperties["api"]; parent != "" {
			return c.BaseResource.List(ctx, request)
		}
	}

	cfg := config.FromTargetConfig(request.TargetConfig, c.Config.Deps())
	if cfg.Project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, c.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	// Apis are global, so there is exactly one collection to walk.
	apisURL := fmt.Sprintf("%s/projects/%s/locations/%s/apis", c.APIConfig.BaseURL, cfg.Project, globalLocation)
	apis, err := c.listNames(ctx, client, apisURL, "apis")
	if err != nil {
		return nil, fmt.Errorf("failed to list api gateway apis: %w", err)
	}

	nativeIDs := make([]string, 0, len(apis))
	var lastErr error
	failed := 0
	for _, api := range apis {
		names, err := c.listNames(ctx, client,
			fmt.Sprintf("%s/%s/configs", c.APIConfig.BaseURL, api), "apiConfigs")
		if err != nil {
			// A shared project holds apis this target does not own, and one
			// still creating refuses the call.
			lastErr = err
			failed++
			continue
		}
		nativeIDs = append(nativeIDs, names...)
	}

	// Skipping one unreadable api is right; skipping every one and reporting an
	// empty list is not - that is indistinguishable from "nothing exists".
	if len(nativeIDs) == 0 && failed > 0 && failed == len(apis) {
		return nil, fmt.Errorf("could not list configs on any of %d apis: %w", failed, lastErr)
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// Status reads the config back once the operation finishes. A create is a
// long-running operation whose response carries the config in its basic form,
// without the OpenAPI documents that define it - only a read under the full
// view returns those, so reporting the operation's own answer left the
// documents missing from the stored state.
func (c *configWalkingProvisioner) Status(
	ctx context.Context,
	request *resource.StatusRequest,
) (*resource.StatusResult, error) {
	return base.StatusWithRead(ctx, c.BaseResource, c.Read, request)
}

func (c *configWalkingProvisioner) listNames(
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
