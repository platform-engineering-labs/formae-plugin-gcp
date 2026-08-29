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

// objectAclProvisioner lists an object's ACL entries by walking the buckets and
// then the objects that hold them.
//
// An object ACL is addressed by two parents at once - /b/{bucket}/o/{object}/acl
// - and discovery lists with neither to name. The bucket ACL walker cannot serve
// this: it stops one level short. Listing each bucket's objects with
// projection=full carries every object's acl inline, so one request per bucket
// is enough rather than one per object.
type objectAclProvisioner struct {
	*base.BaseResource
}

func (o *objectAclProvisioner) List(
	ctx context.Context,
	request *resource.ListRequest,
) (*resource.ListResult, error) {
	// A caller that names both parents wants only that object; the base path
	// builder already composes them.
	if request.AdditionalProperties != nil {
		if request.AdditionalProperties["bucket"] != "" && request.AdditionalProperties["object"] != "" {
			return o.BaseResource.List(ctx, request)
		}
	}

	cfg := config.FromTargetConfig(request.TargetConfig, o.Config.Deps())
	if cfg.Project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, o.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	// The bucket walk is identical to the bucket ACL walker's, so it is reused
	// rather than copied.
	buckets, err := (&aclProvisioner{BaseResource: o.BaseResource}).listBuckets(ctx, client, cfg.Project)
	if err != nil {
		return nil, fmt.Errorf("failed to list buckets: %w", err)
	}

	var nativeIDs []string
	var lastErr error
	failed := 0
	for _, bucket := range buckets {
		ids, err := o.listBucketObjectACLs(ctx, client, bucket)
		if err != nil {
			// A bucket with uniform bucket-level access rejects ACL reads
			// outright, and a shared project holds buckets this target does not
			// own. Skip it rather than letting one hide every other bucket.
			lastErr = err
			failed++
			continue
		}
		nativeIDs = append(nativeIDs, ids...)
	}

	// Skipping an unreadable bucket is right; skipping every one of them and
	// reporting an empty list is not - that is indistinguishable from "nothing
	// exists" and impossible to diagnose.
	if len(nativeIDs) == 0 && failed > 0 && failed == len(buckets) {
		return nil, fmt.Errorf("could not read object ACLs on any of %d buckets: %w", failed, lastErr)
	}
	if nativeIDs == nil {
		nativeIDs = []string{}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// listBucketObjectACLs returns a native ID per ACL entry of every object in one
// bucket. projection=full is what makes the acl array present at all; without it
// the objects list omits it entirely.
func (o *objectAclProvisioner) listBucketObjectACLs(
	ctx context.Context, client *transport.Client, bucket string,
) ([]string, error) {
	var out []string
	listURL := fmt.Sprintf("%s/b/%s/o?projection=full", o.APIConfig.BaseURL, bucket)
	next := listURL
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
			name, _ := item["name"].(string)
			if name == "" {
				continue
			}
			acl, _ := item["acl"].([]interface{})
			for _, rawEntry := range acl {
				entry, ok := rawEntry.(map[string]interface{})
				if !ok {
					continue
				}
				// The entity identifies an entry - there is no name - so it is
				// the last segment of the native ID by construction.
				entity, _ := entry["entity"].(string)
				if entity == "" || strings.Contains(entity, "/") {
					continue
				}
				out = append(out, fmt.Sprintf("b/%s/o/%s/acl/%s", bucket, name, entity))
			}
		}
		token, _ := response.Body["nextPageToken"].(string)
		if token == "" {
			break
		}
		if next, err = transport.AddQueryParam(listURL, "pageToken", token); err != nil {
			return nil, err
		}
	}
	return out, nil
}
