// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
)

// listComputeCollectionNames names every item of a Compute collection.
//
// Sub-resources that live *inside* a parent object - a policy's rules and
// associations, a router's interfaces, named sets and route policies - have no
// collection URL of their own. Discovery lists with no properties, so it can
// name no parent to look in, and the only way to discover such a resource is to
// walk its parents first. Each of those Lists starts here.
func listComputeCollectionNames(
	ctx context.Context, client *transport.Client, url, what string,
) ([]string, error) {
	resp, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: url})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to list "+what)
		return nil, fmt.Errorf("%s", wrapped.Message)
	}
	items, _ := resp.Body["items"].([]interface{})
	names := make([]string, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if name, _ := item["name"].(string); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}
