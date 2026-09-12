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
	"github.com/platform-engineering-labs/formae/pkg/plugin"
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
	cfg := config.FromTargetConfig(request.TargetConfig, a.Config.Deps())
	if cfg.Project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, a.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	// A caller that names its bucket wants only that one, and discovery does:
	// it walks the buckets it found and asks for each one's ACLs. A uniform
	// bucket answers that with 400, so its setting is read first and the ACL
	// collection is only asked for on a bucket that can hold one.
	if request.AdditionalProperties != nil {
		if bucket := request.AdditionalProperties["bucket"]; bucket != "" {
			uniform, err := a.bucketIsUniform(ctx, client, bucket)
			if err != nil {
				return nil, err
			}
			if uniform {
				plugin.LoggerFromContext(ctx).Info("bucket has uniform bucket-level access, no legacy ACLs to list",
					"resourceType", a.ResourceConfig.ResourceType, "bucket", bucket)
				return &resource.ListResult{NativeIDs: []string{}}, nil
			}
			return a.BaseResource.List(ctx, request)
		}
	}

	items, err := a.listBucketItems(ctx, client, cfg.Project)
	if err != nil {
		return nil, fmt.Errorf("failed to list buckets: %w", err)
	}
	buckets := bucketsWithLegacyACLs(items)

	collection := a.ResourceConfig.ResourceType
	nativeIDs := make([]string, 0, len(buckets))
	var lastErr error
	failed := 0
	for _, bucket := range buckets {
		entities, err := a.listEntities(ctx, client, bucket, collection)
		if err != nil {
			// A bucket with uniform bucket-level access rejects ACL reads
			// outright, and a shared project holds buckets this target does not
			// own. Skip it rather than letting one hide every other bucket's
			// entries.
			lastErr = err
			failed++
			continue
		}
		for _, entity := range entities {
			nativeIDs = append(nativeIDs, fmt.Sprintf("b/%s/%s/%s", bucket, collection, entity))
		}
	}

	// Skipping an unreadable bucket is right; skipping every one of them and
	// reporting an empty list is not. That turns a broken walk into "nothing
	// exists", which is indistinguishable from success and impossible to
	// diagnose - so say what went wrong instead.
	if len(nativeIDs) == 0 && failed > 0 && failed == len(buckets) {
		return nil, fmt.Errorf("could not read %s on any of %d buckets: %w",
			collection, failed, lastErr)
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// bucketIsUniform reads one bucket's IAM configuration and reports whether
// uniform bucket-level access is on.
func (a *aclProvisioner) bucketIsUniform(
	ctx context.Context, client *transport.Client, bucket string,
) (bool, error) {
	response, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    fmt.Sprintf("%s/b/%s?fields=iamConfiguration", a.APIConfig.BaseURL, bucket),
	})
	if err != nil {
		return false, transport.WrapError(err, "failed to read bucket "+bucket)
	}
	return uniformBucketLevelAccess(response.Body), nil
}

// listBuckets names every bucket in the project; the notification walker
// wants all of them, since a notification config lives on any bucket.
func (a *aclProvisioner) listBuckets(
	ctx context.Context, client *transport.Client, project string,
) ([]string, error) {
	items, err := a.listBucketItems(ctx, client, project)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(items))
	for _, raw := range items {
		if item, ok := raw.(map[string]interface{}); ok {
			if name, ok := item["name"].(string); ok && name != "" {
				out = append(out, name)
			}
		}
	}
	return out, nil
}

// listBucketItems returns the project's buckets as the API describes them,
// across every page.
func (a *aclProvisioner) listBucketItems(
	ctx context.Context, client *transport.Client, project string,
) ([]interface{}, error) {
	var out []interface{}
	url := fmt.Sprintf("%s/b?project=%s", a.APIConfig.BaseURL, project)
	next := url
	for next != "" {
		response, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: next})
		if err != nil {
			return nil, err
		}
		items, _ := response.Body["items"].([]interface{})
		out = append(out, items...)
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

// bucketsWithLegacyACLs names the buckets on which an ACL can exist at all.
//
// A bucket with uniform bucket-level access has no legacy ACLs by definition,
// and Cloud Storage answers its ACL collections with 400 rather than an empty
// list. Asking is a wasted request that reads as a failure, and a project
// where every bucket is uniform, which is the default for a new bucket, would
// trip the walk's every-bucket-failed guard on every discovery cycle. The
// bucket listing already carries the setting, under its current name and the
// one it had before the rename, so those buckets are left out here.
func bucketsWithLegacyACLs(items []interface{}) []string {
	out := make([]string, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, ok := item["name"].(string)
		if !ok || name == "" {
			continue
		}
		if uniformBucketLevelAccess(item) {
			continue
		}
		out = append(out, name)
	}
	return out
}

func uniformBucketLevelAccess(bucket map[string]interface{}) bool {
	iam, _ := bucket["iamConfiguration"].(map[string]interface{})
	for _, key := range []string{"uniformBucketLevelAccess", "bucketPolicyOnly"} {
		if setting, ok := iam[key].(map[string]interface{}); ok {
			if enabled, ok := setting["enabled"].(bool); ok && enabled {
				return true
			}
		}
	}
	return false
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

// notificationProvisioner lists bucket notifications by walking the buckets
// that hold them.
//
// A notification lives at /b/{bucket}/notificationConfigs and Cloud Storage has
// no endpoint spanning buckets, while discovery lists with no parent to name -
// the same shape as the ACL types above, and the reason a notification could
// never be discovered.
type notificationProvisioner struct {
	*base.BaseResource
}

func (n *notificationProvisioner) List(
	ctx context.Context,
	request *resource.ListRequest,
) (*resource.ListResult, error) {
	// A caller that names its bucket wants only that one.
	if request.AdditionalProperties != nil {
		if parent := request.AdditionalProperties["bucket"]; parent != "" {
			return n.BaseResource.List(ctx, request)
		}
	}

	cfg := config.FromTargetConfig(request.TargetConfig, n.Config.Deps())
	if cfg.Project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, n.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	buckets, err := (&aclProvisioner{BaseResource: n.BaseResource}).listBuckets(ctx, client, cfg.Project)
	if err != nil {
		return nil, fmt.Errorf("failed to list buckets: %w", err)
	}

	var nativeIDs []string
	var lastErr error
	failed := 0
	for _, bucket := range buckets {
		response, err := client.SendRequest(ctx, transport.RequestOptions{
			Method: "GET",
			URL:    fmt.Sprintf("%s/b/%s/notificationConfigs", n.APIConfig.BaseURL, bucket),
		})
		if err != nil {
			// A shared project holds buckets this target does not own.
			lastErr = err
			failed++
			continue
		}
		items, _ := response.Body["items"].([]interface{})
		for _, raw := range items {
			item, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			// The id is server-assigned and is what addresses the notification.
			if id, ok := item["id"].(string); ok && id != "" {
				nativeIDs = append(nativeIDs, fmt.Sprintf("b/%s/notificationConfigs/%s", bucket, id))
			}
		}
	}

	// Skipping one unreadable bucket is right; skipping every one and reporting
	// an empty list is not - that is indistinguishable from "nothing exists".
	if len(nativeIDs) == 0 && failed > 0 && failed == len(buckets) {
		return nil, fmt.Errorf("could not read notifications on any of %d buckets: %w", failed, lastErr)
	}
	if nativeIDs == nil {
		nativeIDs = []string{}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}
