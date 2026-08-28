// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dns

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// A record set lives in a managed zone, and discovery lists with no properties,
// so it can name no zone to look in. Cloud DNS has no wildcard for the segment,
// so the only way to discover one is to walk the zones.
//
// Two things differ from the other walkers in this plugin: a record set is
// addressed by name *and* type, so the native ID carries both; and every zone
// is born with an SOA and an NS record set that nobody declared, which
// discovery will surface as unmanaged.
type resourceRecordSetListProvisioner struct {
	prov.Provisioner
	cfg *config.Config
}

// registerResourceRecordSetList is called from the package init in resources.go
// so the generic registration is guaranteed to have landed first.
func registerResourceRecordSetList() {
	registry.Register(ResourceRecordSetResourceType,
		[]resource.Operation{resource.OperationList},
		func(cfg *config.Config) prov.Provisioner {
			return &resourceRecordSetListProvisioner{
				Provisioner: dnsRegistry.CreateProvisioner(cfg, ResourceRecordSetResourceType),
				cfg:         cfg,
			}
		})
}

func (p *resourceRecordSetListProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	if request.AdditionalProperties != nil && request.AdditionalProperties["managedZone"] != "" {
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

	zonesURL := fmt.Sprintf("%s/projects/%s/managedZones", DNSAPI.BaseURL, cfg.Project)
	resp, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: zonesURL})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to list DNS managed zones")
		return nil, fmt.Errorf("%s", wrapped.Message)
	}

	nativeIDs := []string{}
	zones, _ := resp.Body["managedZones"].([]interface{})
	for _, raw := range zones {
		zone, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		zoneName, _ := zone["name"].(string)
		if zoneName == "" {
			continue
		}
		rrsetsResp, listErr := client.SendRequest(ctx, transport.RequestOptions{
			Method: "GET",
			URL:    fmt.Sprintf("%s/%s/rrsets", zonesURL, zoneName),
		})
		if listErr != nil {
			// One unreadable zone must not hide the rest.
			continue
		}
		rrsets, _ := rrsetsResp.Body["rrsets"].([]interface{})
		for _, rawSet := range rrsets {
			set, ok := rawSet.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := set["name"].(string)
			recordType, _ := set["type"].(string)
			if name == "" || recordType == "" {
				continue
			}
			nativeIDs = append(nativeIDs, fmt.Sprintf(
				"projects/%s/managedZones/%s/rrsets/%s/%s", cfg.Project, zoneName, name, recordType))
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}
