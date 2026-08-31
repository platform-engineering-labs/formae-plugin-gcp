// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dns

import (
	"context"
	"encoding/json"
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
	registry.Register(ResourceRecordSetResourceType,
		[]resource.Operation{resource.OperationRead},
		func(cfg *config.Config) prov.Provisioner {
			return &resourceRecordSetReadProvisioner{
				Provisioner: dnsRegistry.CreateProvisioner(cfg, ResourceRecordSetResourceType),
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

// resourceRecordSetReadProvisioner adds back the zone a record set belongs to.
//
// Cloud DNS answers a record-set read with name, type, ttl and rrdatas, and
// nothing naming the managed zone - that lives only in the URL. So a discovered
// record set arrived without `managedZone`, a required createOnly property, and
// was dropped instead of entering inventory: the walk found it on every pass
// while Discover timed out.
//
// This is the same defect the response policy rule had, for the same reason,
// and it needs the same shape of fix: TransformContext carries no parent, so
// the zone has to come from the native ID.
type resourceRecordSetReadProvisioner struct {
	prov.Provisioner
}

func (p *resourceRecordSetReadProvisioner) Read(
	ctx context.Context, request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	result, err := p.Provisioner.Read(ctx, request)
	if err != nil || result == nil || result.ErrorCode != "" || result.Properties == "" {
		return result, err
	}

	pathCtx, parseErr := parseDNSNativeID(request.NativeID)
	if parseErr != nil || pathCtx.ParentResource == "" {
		return result, nil
	}

	var props map[string]interface{}
	if unmarshalErr := json.Unmarshal([]byte(result.Properties), &props); unmarshalErr != nil {
		return result, nil
	}
	if _, present := props["managedZone"]; present {
		return result, nil
	}
	props["managedZone"] = pathCtx.ParentResource

	enriched, marshalErr := json.Marshal(props)
	if marshalErr != nil {
		return result, nil
	}
	result.Properties = string(enriched)
	return result, nil
}
