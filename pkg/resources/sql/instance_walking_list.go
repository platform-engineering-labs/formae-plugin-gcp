// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package sql

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

// Every sqladmin resource except the instance itself lives under one, and
// discovery lists with no properties - so it can name no instance to look in,
// and sqladmin has no wildcard standing in for "all of them". The only way to
// discover any of them is to walk the instances first.
//
// Four resource types need exactly this, differing only in which collection to
// ask for and how an item names itself, so they share one walker rather than
// four copies of it.
type instanceWalkingListProvisioner struct {
	prov.Provisioner
	cfg *config.Config
	// collection is the sub-collection to list under each instance.
	collection string
	// idOf returns the segment that addresses one item, which is not always its
	// name: a certificate is addressed by fingerprint and a backup run by a
	// server-assigned id.
	idOf func(map[string]interface{}) string
}

// registerInstanceWalkingLists replaces the List entry for every instance-scoped
// type. It is called from the package init in resources.go rather than from an
// init here, so it cannot matter whether this file sorts before or after
// resources.go - the generic registration must land first, or there would be no
// provisioner to wrap.
func registerInstanceWalkingLists() {
	byName := func(item map[string]interface{}) string { return utils.GetString(item, "name") }

	for _, spec := range []struct {
		resourceType string
		collection   string
		idOf         func(map[string]interface{}) string
	}{
		{UserResourceType, "users", byName},
		// Databases predate this batch and had no List override, so discovery
		// asked a collection URL with no instance in it and found nothing.
		{DatabaseResourceType, "databases", byName},
		{SslCertResourceType, "sslCerts", sslCertFingerprint},
		{BackupRunResourceType, "backupRuns", backupRunID},
	} {
		spec := spec
		registry.Register(spec.resourceType,
			[]resource.Operation{resource.OperationList},
			func(cfg *config.Config) prov.Provisioner {
				return &instanceWalkingListProvisioner{
					Provisioner: sqlRegistry.CreateProvisioner(cfg, spec.resourceType),
					cfg:         cfg,
					collection:  spec.collection,
					idOf:        spec.idOf,
				}
			})
	}
}

func (p *instanceWalkingListProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	// A named instance is the caller telling us where to look; the generic
	// implementation already handles that.
	if request.AdditionalProperties != nil && request.AdditionalProperties["instance"] != "" {
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

	instancesURL := fmt.Sprintf("%s/projects/%s/instances", SQLAPI.BaseURL, cfg.Project)
	resp, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: instancesURL})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to list SQL instances")
		return nil, fmt.Errorf("%s", wrapped.Message)
	}

	nativeIDs := []string{}
	instances, _ := resp.Body["items"].([]interface{})
	for _, raw := range instances {
		instance, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		instanceName := utils.GetString(instance, "name")
		if instanceName == "" {
			continue
		}
		itemsResp, listErr := client.SendRequest(ctx, transport.RequestOptions{
			Method: "GET",
			URL:    fmt.Sprintf("%s/%s/%s", instancesURL, instanceName, p.collection),
		})
		if listErr != nil {
			// One unreadable instance must not hide the rest.
			continue
		}
		items, _ := itemsResp.Body["items"].([]interface{})
		for _, rawItem := range items {
			item, ok := rawItem.(map[string]interface{})
			if !ok {
				continue
			}
			if id := p.idOf(item); id != "" {
				nativeIDs = append(nativeIDs, fmt.Sprintf("projects/%s/instances/%s/%s/%s",
					cfg.Project, instanceName, p.collection, id))
			}
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}
