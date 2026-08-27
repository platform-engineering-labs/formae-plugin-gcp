// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package logging

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

// logViewListProvisioner overrides List for log views and delegates everything
// else to the generic provisioner.
//
// A view lives under a bucket, and discovery lists with no properties, so it can
// name no bucket to look in. Cloud Logging accepts the "-" wildcard for a
// bucket's *location* but not for the bucket itself, so the only way to discover
// a view is to walk the buckets first.
type logViewListProvisioner struct {
	prov.Provisioner
	cfg *config.Config
}

// registerLogViewListOverride replaces only the List entry; create, read, update
// and delete keep the generic implementation.
//
// It is called at the end of the package's init rather than from an init() of
// its own: Go runs a package's init functions in filename order, and this file
// sorts before resources.go, so an init() here would be undone by the generic
// registration that follows it.
func registerLogViewListOverride() {
	registry.Register(LogViewResourceType,
		[]resource.Operation{resource.OperationList},
		func(cfg *config.Config) prov.Provisioner {
			return &logViewListProvisioner{
				Provisioner: loggingRegistry.CreateProvisioner(cfg, LogViewResourceType),
				cfg:         cfg,
			}
		})
}

func (p *logViewListProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	// A named bucket is the caller telling us where to look; the generic
	// implementation already handles that.
	if request.AdditionalProperties != nil && request.AdditionalProperties["bucket"] != "" {
		return p.Provisioner.List(ctx, request)
	}

	cfg := config.PathFromTargetConfig(request.TargetConfig)
	project := cfg.Project
	if project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, p.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	// "locations/-" spans every region, so a view is found wherever its bucket
	// lives rather than only in the target's configured location.
	bucketsURL := fmt.Sprintf("%s/projects/%s/locations/-/buckets",
		LoggingViewAPI.BaseURL, project)
	resp, rErr := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: bucketsURL})
	if rErr != nil {
		wrapped := transport.WrapError(rErr, "failed to list logging buckets")
		return nil, fmt.Errorf("%s", wrapped.Message)
	}

	nativeIDs := []string{}
	buckets, _ := resp.Body["buckets"].([]interface{})
	for _, raw := range buckets {
		bucket, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		// A bucket reports its full path, which is the views collection's parent.
		fullName, _ := bucket["name"].(string)
		if !strings.Contains(fullName, "/buckets/") {
			continue
		}
		viewsResp, listErr := client.SendRequest(ctx, transport.RequestOptions{
			Method: "GET",
			URL:    fmt.Sprintf("%s/%s/views", LoggingViewAPI.BaseURL, fullName),
		})
		if listErr != nil {
			// One unreadable bucket must not hide the rest.
			continue
		}
		views, _ := viewsResp.Body["views"].([]interface{})
		for _, rawView := range views {
			view, ok := rawView.(map[string]interface{})
			if !ok {
				continue
			}
			// A view reports its full path, which is already the native ID shape.
			if name, _ := view["name"].(string); strings.Contains(name, "/views/") {
				nativeIDs = append(nativeIDs, name)
			}
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}
