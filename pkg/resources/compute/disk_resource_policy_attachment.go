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
type DiskResourcePolicyAttachmentProvisioner struct {
	*base.BaseResource
}

var _ prov.Provisioner = (*DiskResourcePolicyAttachmentProvisioner)(nil)

func init() {
	registry.Register(DiskResourcePolicyAttachmentResourceType,
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
						Scope:        &base.ScopeConfig{Type: base.ScopeZonal},
					},
					NativeIDConfig: ComputeNativeID,
				},
			}
		})
}

// buildAttachmentNativeID composes
// "projects/{p}/zones/{zone}/disks/{disk}/resourcePolicies/{policy}".
func buildAttachmentNativeID(project, zone, disk, policy string) string {
	return fmt.Sprintf("projects/%s/zones/%s/disks/%s/resourcePolicies/%s",
		project, zone, disk, policy)
}

// parseAttachmentNativeID splits the composite id. An attachment has no identity
// beyond the (disk, policy) pair, so all four parts must survive.
func parseAttachmentNativeID(nativeID string) (project, zone, disk, policy string, err error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 8 || parts[0] != "projects" || parts[2] != "zones" ||
		parts[4] != "disks" || parts[6] != "resourcePolicies" {
		return "", "", "", "", fmt.Errorf("invalid disk resource policy attachment native ID: %s", nativeID)
	}
	return parts[1], parts[3], parts[5], parts[7], nil
}

func (p *DiskResourcePolicyAttachmentProvisioner) diskURL(project, zone, disk string) string {
	return fmt.Sprintf("%s/projects/%s/zones/%s/disks/%s",
		p.APIConfig.BaseURL, project, zone, disk)
}

// policyURL expands a bare policy name into the regional resource path the verbs
// expect. The region is derived from the zone, since a policy must live in the
// disk's region.
func policyURLFor(project, zone, policy string) string {
	if strings.Contains(policy, "/") {
		return policy
	}
	region := zone
	if i := strings.LastIndex(zone, "-"); i > 0 {
		region = zone[:i]
	}
	return fmt.Sprintf("projects/%s/regions/%s/resourcePolicies/%s", project, region, policy)
}

func (p *DiskResourcePolicyAttachmentProvisioner) projectFor(targetConfig json.RawMessage, fallback string) string {
	if cfg := config.FromTargetConfig(targetConfig); cfg != nil && cfg.Project != "" {
		return cfg.Project
	}
	return fallback
}

func (p *DiskResourcePolicyAttachmentProvisioner) zoneFor(targetConfig json.RawMessage, fallback string) string {
	if cfg := config.FromTargetConfig(targetConfig); cfg != nil && cfg.Zone != "" {
		return cfg.Zone
	}
	return fallback
}

func (p *DiskResourcePolicyAttachmentProvisioner) issueVerb(
	ctx context.Context, url, policy, project, zone string,
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
	return p.OperationConfig.OperationURLBuilder(
		base.PathContext{Project: project, Zone: zone}, opID), nil
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
	zone := p.zoneFor(request.TargetConfig, "")
	if z, ok := props["zone"].(string); ok && z != "" {
		zone = z
	}
	if project == "" || zone == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"target project and zone are required"), nil
	}

	requestID, verbErr := p.issueVerb(ctx,
		p.diskURL(project, zone, disk)+"/addResourcePolicies",
		policyURLFor(project, zone, name), project, zone)
	if verbErr != nil {
		return createFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        buildAttachmentNativeID(project, zone, disk, name),
			RequestID:       requestID,
			StatusMessage:   "disk resource policy attachment in progress",
		},
	}, nil
}

func (p *DiskResourcePolicyAttachmentProvisioner) Read(
	ctx context.Context, request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	project, zone, disk, policy, err := parseAttachmentNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}
	project = p.projectFor(request.TargetConfig, project)

	client, cErr := transport.NewClient(ctx, p.Config)
	if cErr != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", cErr)
	}
	resp, rErr := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET", URL: p.diskURL(project, zone, disk),
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
		"name": policy,
		"disk": disk,
		"zone": zone,
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
	project, zone, disk, policy, err := parseAttachmentNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	project = p.projectFor(request.TargetConfig, project)

	requestID, verbErr := p.issueVerb(ctx,
		p.diskURL(project, zone, disk)+"/removeResourcePolicies",
		policyURLFor(project, zone, policy), project, zone)
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

// List needs a disk to look inside, so without one there is nothing to
// enumerate; an empty result is the honest answer.
func (p *DiskResourcePolicyAttachmentProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	return &resource.ListResult{NativeIDs: []string{}}, nil
}

// policyNameOf reduces a resource policy URL to its bare name.
func policyNameOf(url string) string {
	if i := strings.LastIndex(url, "/"); i >= 0 {
		return url[i+1:]
	}
	return url
}
