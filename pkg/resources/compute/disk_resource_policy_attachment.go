// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// DiskResourcePolicyAttachmentProvisioner attaches one resource policy to one
// disk. The attachment is not a REST resource: it is an entry in the disk's
// "resourcePolicies" array, added and removed with the addResourcePolicies /
// removeResourcePolicies verbs. Without it a `resourcePolicy` snapshot schedule
// is inert - it has nothing to snapshot.
//
// There is nothing to update: the attachment is just a pair, so a change means
// detach and attach.
//
// Zonal and regional disks expose the same two verbs, differing only in whether
// the disk sits under zones/{zone} or regions/{region}, so both are one
// provisioner carrying a `regional` flag.
type DiskResourcePolicyAttachmentProvisioner struct {
	*base.BaseResource
	regional bool
}

var _ prov.Provisioner = (*DiskResourcePolicyAttachmentProvisioner)(nil)

func init() {
	registerDiskAttachment(DiskResourcePolicyAttachmentResourceType, false)
	registerDiskAttachment(RegionDiskResourcePolicyAttachmentResourceType, true)
}

func registerDiskAttachment(resourceType string, regional bool) {
	scope := base.ScopeZonal
	if regional {
		scope = base.ScopeRegional
	}
	registry.Register(resourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(cfg *config.Config) prov.Provisioner {
			return &DiskResourcePolicyAttachmentProvisioner{
				BaseResource: &base.BaseResource{
					Config:          cfg,
					APIConfig:       ComputeAPI,
					OperationConfig: ComputeOperations,
					ResourceConfig: base.ResourceConfig{
						ResourceType: "disks",
						Scope:        &base.ScopeConfig{Type: scope},
					},
					NativeIDConfig: ComputeNativeID,
				},
				regional: regional,
			}
		})
}

// locationSegment is "zones/{zone}" or "regions/{region}" — a regional disk
// lives in a region, and the verbs sit under the disk either way.
func (p *DiskResourcePolicyAttachmentProvisioner) locationSegment(location string) string {
	if p.regional {
		return "regions/" + location
	}
	return "zones/" + location
}

// locationKey is the property (and native ID segment) naming the disk's home:
// a zone for a zonal disk, a region for a regional one.
func (p *DiskResourcePolicyAttachmentProvisioner) locationKey() string {
	if p.regional {
		return "region"
	}
	return "zone"
}

// buildAttachmentNativeID composes
// "projects/{p}/{zones|regions}/{location}/disks/{disk}/resourcePolicies/{policy}".
func (p *DiskResourcePolicyAttachmentProvisioner) buildAttachmentNativeID(project, location, disk, policy string) string {
	return fmt.Sprintf("projects/%s/%s/disks/%s/resourcePolicies/%s",
		project, p.locationSegment(location), disk, policy)
}

// parseAttachmentNativeID splits the composite id. An attachment has no identity
// beyond the (disk, policy) pair, so all four parts must survive. The scope
// segment must match this provisioner's kind, or a regional attachment could be
// read against a zonal URL.
func (p *DiskResourcePolicyAttachmentProvisioner) parseAttachmentNativeID(nativeID string) (project, location, disk, policy string, err error) {
	wantScope := "zones"
	if p.regional {
		wantScope = "regions"
	}
	parts := strings.Split(nativeID, "/")
	if len(parts) != 8 || parts[0] != "projects" || parts[2] != wantScope ||
		parts[3] == "" || parts[4] != "disks" || parts[6] != "resourcePolicies" {
		return "", "", "", "", fmt.Errorf("invalid disk resource policy attachment native ID: %s", nativeID)
	}
	return parts[1], parts[3], parts[5], parts[7], nil
}

func (p *DiskResourcePolicyAttachmentProvisioner) diskURL(project, location, disk string) string {
	return fmt.Sprintf("%s/projects/%s/%s/disks/%s",
		p.APIConfig.BaseURL, project, p.locationSegment(location), disk)
}

// policyURL expands a bare policy name into the regional resource path the verbs
// expect. The region is derived from the zone, since a policy must live in the
// disk's region.
func policyURLFor(project, zone, policy string) string {
	if strings.Contains(policy, "/") {
		return policy
	}
	return fmt.Sprintf("projects/%s/regions/%s/resourcePolicies/%s",
		project, regionOfZone(zone), policy)
}

// regionOfZone drops a zone's trailing letter. A region is returned unchanged,
// so a regional disk's location passes straight through.
func regionOfZone(zone string) string {
	parts := strings.Split(zone, "-")
	if len(parts) == 3 {
		return strings.Join(parts[:2], "-")
	}
	return zone
}

func (p *DiskResourcePolicyAttachmentProvisioner) projectFor(targetConfig json.RawMessage, fallback string) string {
	if cfg := config.PathFromTargetConfig(targetConfig); cfg.Project != "" {
		return cfg.Project
	}
	return fallback
}

// locationFor prefers the target's configured zone or region, depending on kind.
func (p *DiskResourcePolicyAttachmentProvisioner) locationFor(targetConfig json.RawMessage, fallback string) string {
	cfg := config.PathFromTargetConfig(targetConfig)
	if p.regional && cfg.Region != "" {
		return cfg.Region
	}
	if !p.regional && cfg.Zone != "" {
		return cfg.Zone
	}
	return fallback
}

func (p *DiskResourcePolicyAttachmentProvisioner) issueVerb(
	ctx context.Context, url, policy, project, location string,
) (string, *transport.Error) {
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return "", transport.WrapError(err, "failed to create transport client")
	}
	resp, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "POST",
		URL:    url,
		Body:   map[string]interface{}{"resourcePolicies": []interface{}{policy}},
	})
	if err != nil {
		return "", transport.WrapError(err, "disk resource policy verb failed")
	}
	opID := p.OperationConfig.OperationIDExtractor(resp.Body)
	pathCtx := base.PathContext{Project: project, Zone: location}
	if p.regional {
		pathCtx = base.PathContext{Project: project, Region: location}
	}
	return p.OperationConfig.OperationURLBuilder(pathCtx, opID), nil
}

func (p *DiskResourcePolicyAttachmentProvisioner) Create(
	ctx context.Context, request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid properties: %v", err)), nil
	}
	disk, _ := props["disk"].(string)
	name, _ := props["name"].(string)
	if disk == "" || name == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"name (the policy) and disk are required"), nil
	}
	project := p.projectFor(request.TargetConfig, "")
	location := p.locationFor(request.TargetConfig, "")
	if l, ok := props[p.locationKey()].(string); ok && l != "" {
		location = l
	}
	if project == "" || location == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("target project and %s are required", p.locationKey())), nil
	}

	requestID, verbErr := p.issueVerb(ctx,
		p.diskURL(project, location, disk)+"/addResourcePolicies",
		policyURLFor(project, location, name), project, location)
	if verbErr != nil {
		return createFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        p.buildAttachmentNativeID(project, location, disk, name),
			RequestID:       requestID,
			StatusMessage:   "disk resource policy attachment in progress",
		},
	}, nil
}

func (p *DiskResourcePolicyAttachmentProvisioner) Read(
	ctx context.Context, request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	project, location, disk, policy, err := p.parseAttachmentNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}
	project = p.projectFor(request.TargetConfig, project)

	client, cErr := transport.NewClient(ctx, p.Config)
	if cErr != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", cErr)
	}
	resp, rErr := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET", URL: p.diskURL(project, location, disk),
	})
	if rErr != nil {
		wrapped := transport.WrapError(rErr, "failed to read disk")
		return &resource.ReadResult{
			ErrorCode: transport.ToResourceErrorCode(wrapped.Code),
		}, nil
	}

	// The attachment exists only as an entry in the disk's list; if the entry is
	// gone the attachment is gone, and formae must be told so rather than see a
	// generic failure.
	attached := false
	if list, ok := resp.Body["resourcePolicies"].([]interface{}); ok {
		for _, item := range list {
			if url, ok := item.(string); ok && policyNameOf(url) == policy {
				attached = true
				break
			}
		}
	}
	if !attached {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
	}

	encoded, mErr := json.Marshal(map[string]interface{}{
		"name":          policy,
		"disk":          disk,
		p.locationKey(): location,
	})
	if mErr != nil {
		return nil, fmt.Errorf("failed to marshal attachment properties: %w", mErr)
	}
	return &resource.ReadResult{Properties: string(encoded)}, nil
}

func (p *DiskResourcePolicyAttachmentProvisioner) Update(
	ctx context.Context, request *resource.UpdateRequest,
) (*resource.UpdateResult, error) {
	return updateFailure(resource.OperationErrorCodeNotUpdatable,
		"a disk resource policy attachment is a (disk, policy) pair; a change replaces it"), nil
}

func (p *DiskResourcePolicyAttachmentProvisioner) Delete(
	ctx context.Context, request *resource.DeleteRequest,
) (*resource.DeleteResult, error) {
	project, location, disk, policy, err := p.parseAttachmentNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	project = p.projectFor(request.TargetConfig, project)

	requestID, verbErr := p.issueVerb(ctx,
		p.diskURL(project, location, disk)+"/removeResourcePolicies",
		policyURLFor(project, location, policy), project, location)
	if verbErr != nil {
		return deleteFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        request.NativeID,
			RequestID:       requestID,
			StatusMessage:   "disk resource policy detachment in progress",
		},
	}, nil
}

// List walks the project's disks and reports every policy attached to them.
// Discovery calls this with no hints, so returning nothing would mean an
// attachment can be managed but never discovered. The aggregated endpoint covers
// every zone in one call.
func (p *DiskResourcePolicyAttachmentProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	project := p.projectFor(request.TargetConfig, "")
	if project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}
	resp, rErr := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    fmt.Sprintf("%s/projects/%s/aggregated/disks", p.APIConfig.BaseURL, project),
	})
	if rErr != nil {
		wrapped := transport.WrapError(rErr, "failed to list disks")
		return nil, fmt.Errorf("%s", wrapped.Message)
	}

	// Keys are "zones/{zone}" or "regions/{region}". Each kind reports only its
	// own scope: a regional id emitted by the zonal kind (or the reverse) would
	// 404 on read.
	wantPrefix := "zones/"
	if p.regional {
		wantPrefix = "regions/"
	}
	nativeIDs := []string{}
	scopes, _ := resp.Body["items"].(map[string]interface{})
	for scope, payload := range scopes {
		if !strings.HasPrefix(scope, wantPrefix) {
			continue
		}
		location := strings.TrimPrefix(scope, wantPrefix)
		entry, ok := payload.(map[string]interface{})
		if !ok {
			continue
		}
		disks, _ := entry["disks"].([]interface{})
		for _, raw := range disks {
			disk, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			diskName, _ := disk["name"].(string)
			if diskName == "" {
				continue
			}
			policies, _ := disk["resourcePolicies"].([]interface{})
			for _, rawPolicy := range policies {
				policyURL, ok := rawPolicy.(string)
				if !ok || policyURL == "" {
					continue
				}
				nativeIDs = append(nativeIDs,
					p.buildAttachmentNativeID(project, location, diskName, policyNameOf(policyURL)))
			}
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// policyNameOf reduces a resource policy URL to its bare name.
func policyNameOf(url string) string {
	if i := strings.LastIndex(url, "/"); i >= 0 {
		return url[i+1:]
	}
	return url
}

// Status routes through the shared read-back so post-create and post-update
// state carries the resource's real properties, not just what was declared.
func (p *DiskResourcePolicyAttachmentProvisioner) Status(
	ctx context.Context, request *resource.StatusRequest,
) (*resource.StatusResult, error) {
	return base.StatusWithRead(ctx, p.BaseResource, p.Read, request)
}
