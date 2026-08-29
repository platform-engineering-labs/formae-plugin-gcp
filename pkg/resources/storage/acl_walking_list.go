// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// aclProvisioner lists ACL entries by walking the buckets that hold them.
//
// An ACL lives at /b/{bucket}/acl or /b/{bucket}/defaultObjectAcl, and Cloud
// Storage has no endpoint that spans buckets - there is no "-" wildcard in the
// bucket position the way privateca and Datastream offer one. Discovery lists
// with no parent to name, so without this it asks for a URL with an empty
// bucket segment and finds nothing, which made both ACL types undiscoverable.
type aclProvisioner struct {
	*base.BaseResource
}

func (a *aclProvisioner) List(
	ctx context.Context,
	request *resource.ListRequest,
) (*resource.ListResult, error) {
	// A caller that names its bucket wants only that one; the base path builder
	// already handles it.
	if request.AdditionalProperties != nil {
		if parent := request.AdditionalProperties["bucket"]; parent != "" {
			return a.BaseResource.List(ctx, request)
		}
	}

	cfg := config.FromTargetConfig(request.TargetConfig, a.Config.Deps())
	if cfg.Project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, a.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	buckets, err := a.listBuckets(ctx, client, cfg.Project)
	if err != nil {
		return nil, fmt.Errorf("failed to list buckets: %w", err)
	}

	collection := a.ResourceConfig.ResourceType
	nativeIDs := make([]string, 0, len(buckets))
	for _, bucket := range buckets {
		entities, err := a.listEntities(ctx, client, bucket, collection)
		if err != nil {
			// A bucket with uniform bucket-level access rejects ACL reads
			// outright, and a shared project holds buckets this target does not
			// own. Skip it rather than letting one hide every other bucket's
			// entries.
			continue
		}
		for _, entity := range entities {
			nativeIDs = append(nativeIDs, fmt.Sprintf("b/%s/%s/%s", bucket, collection, entity))
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

func (a *aclProvisioner) listBuckets(
	ctx context.Context, client *transport.Client, project string,
) ([]string, error) {
	var out []string
	url := fmt.Sprintf("%s/b?project=%s", a.APIConfig.BaseURL, project)
	next := url
	for next != "" {
		response, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: next})
		if err != nil {
			return nil, err
		}
		items, _ := response.Body["items"].([]interface{})
		for _, raw := range items {
			item, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if name, ok := item["name"].(string); ok && name != "" {
				out = append(out, name)
			}
		}
		token, _ := response.Body["nextPageToken"].(string)
		if token == "" {
			break
		}
		if next, err = transport.AddQueryParam(url, "pageToken", token); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// listEntities returns the entity of every ACL entry on one bucket. The entity
// is what identifies an entry - there is no name - and it can contain slashes
// in principle, so it is the last segment of the native ID by construction.
func (a *aclProvisioner) listEntities(
	ctx context.Context, client *transport.Client, bucket, collection string,
) ([]string, error) {
	response, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    fmt.Sprintf("%s/b/%s/%s", a.APIConfig.BaseURL, bucket, collection),
	})
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
		if entity, ok := item["entity"].(string); ok && entity != "" && !strings.Contains(entity, "/") {
			out = append(out, entity)
		}
	}
	return out, nil
}
