// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package analyticshub

import (
	"context"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// nestedProvisioner lists a collection that only exists underneath a data
// exchange. Analytics Hub has no listable URL spanning exchanges - listings and
// query templates can only be asked for one exchange at a time, and there is no
// "-" wildcard in the parent position - so discovery, which lists with no
// parent to name, would otherwise find nothing at all. Walk the exchanges and
// ask each one.
type nestedProvisioner struct {
	*base.BaseResource
}

// List walks every data exchange in the target's location and concatenates the
// nested collection of each.
func (n *nestedProvisioner) List(
	ctx context.Context,
	request *resource.ListRequest,
) (*resource.ListResult, error) {
	// A caller that names its parent wants only that one; the base path builder
	// already handles it.
	if request.AdditionalProperties != nil {
		if parent := request.AdditionalProperties["dataExchange"]; parent != "" {
			return n.BaseResource.List(ctx, request)
		}
	}

	cfg := config.FromTargetConfig(request.TargetConfig, n.Config.Deps())
	if cfg.Location == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, n.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	base := fmt.Sprintf("%s/projects/%s/locations/%s",
		n.APIConfig.BaseURL, cfg.Project, cfg.Location)

	exchanges, err := collect(ctx, client, base+"/dataExchanges", "dataExchanges")
	if err != nil {
		return nil, fmt.Errorf("failed to list data exchanges: %w", err)
	}

	collection := n.ResourceConfig.ResourceType
	nativeIDs := make([]string, 0, len(exchanges))
	for _, exchange := range exchanges {
		// "exchange" is already a full resource path.
		nested, err := collect(ctx, client, n.APIConfig.BaseURL+"/"+exchange+"/"+collection, collection)
		if err != nil {
			// One unreadable exchange must not hide every other exchange's
			// contents - a shared project holds exchanges this target does not
			// own. Skip it and keep walking.
			continue
		}
		nativeIDs = append(nativeIDs, nested...)
	}

	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// collect GETs one collection URL and returns the full resource path of every
// item, following nextPageToken to the end.
func collect(ctx context.Context, client *transport.Client, url, itemsKey string) ([]string, error) {
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
