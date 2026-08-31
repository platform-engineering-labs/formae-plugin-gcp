// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package storage

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// Both folder types live in a bucket, and discovery lists with no properties -
// so it can name no bucket, the path builder falls through to the
// project-scoped branch and asks /projects/{p}/managedFolders, which addresses
// nothing. GCS has no wildcard for the bucket segment, so the only way to
// discover either is to walk the buckets.
//
// This is the sixth copy of this shape in the plugin (Service Directory,
// Spanner, Cloud SQL, DNS, Bigtable and now Storage). It belongs in base.
type bucketWalkingListProvisioner struct {
	prov.Provisioner
	cfg        *config.Config
	collection string
}

// registerBucketWalkingLists is called from the package init in resources.go so
// the generic registration is guaranteed to have landed first.
func registerBucketWalkingLists() {
	for _, spec := range []struct {
		resourceType string
		collection   string
	}{
		{ManagedFolderResourceType, "managedFolders"},
		{FolderResourceType, "folders"},
	} {
		spec := spec
		registry.Register(spec.resourceType,
			[]resource.Operation{resource.OperationList},
			func(cfg *config.Config) prov.Provisioner {
				return &bucketWalkingListProvisioner{
					Provisioner: storageRegistry.CreateProvisioner(cfg, spec.resourceType),
					cfg:         cfg,
					collection:  spec.collection,
				}
			})
	}
}

func (p *bucketWalkingListProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	// A named bucket is the caller telling us where to look.
	if request.AdditionalProperties != nil && request.AdditionalProperties["bucket"] != "" {
		return p.Provisioner.List(ctx, request)
	}

	cfg := config.PathFromTargetConfig(request.TargetConfig)
	if cfg.Project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, p.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	bucketsURL := fmt.Sprintf("%s/b?project=%s", StorageAPI.BaseURL, cfg.Project)
	resp, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: bucketsURL})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to list storage buckets")
		return nil, fmt.Errorf("%s", wrapped.Message)
	}

	nativeIDs := []string{}
	buckets, _ := resp.Body["items"].([]interface{})
	for _, raw := range buckets {
		bucket, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		bucketName := utils.GetString(bucket, "name")
		if bucketName == "" {
			continue
		}
		itemsResp, listErr := client.SendRequest(ctx, transport.RequestOptions{
			Method: "GET",
			URL:    fmt.Sprintf("%s/b/%s/%s", StorageAPI.BaseURL, bucketName, p.collection),
		})
		if listErr != nil {
			// A flat bucket has no folders and a bucket with ACLs has no managed
			// folders; neither is a reason to hide the rest.
			continue
		}
		items, _ := itemsResp.Body["items"].([]interface{})
		for _, rawItem := range items {
			item, ok := rawItem.(map[string]interface{})
			if !ok {
				continue
			}
			// The name carries its trailing slash, which the native ID keeps.
			if name := utils.GetString(item, "name"); name != "" {
				nativeIDs = append(nativeIDs,
					fmt.Sprintf("b/%s/%s/%s", bucketName, p.collection, name))
			}
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}
