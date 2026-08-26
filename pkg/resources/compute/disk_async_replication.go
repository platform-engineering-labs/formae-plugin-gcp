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

// DiskAsyncReplicationProvisioner manages the replication link between a primary
// disk and a secondary disk in another region — the pairing that makes
// cross-region disaster recovery work. The disks themselves are ordinary
// `Disk` resources; this is the relationship between them.
//
// It is not a REST resource: startAsyncReplication and stopAsyncReplication are
// verbs on the primary disk. The one subtlety worth knowing is that stopping
// replication does not clear `asyncPrimaryDisk` from the secondary — only
// `resourceStatus.asyncPrimaryDisk.state` changes, from ACTIVE to STOPPED. So a
// read has to judge by state, not by whether the field is present, or a stopped
// pair would look like a live one forever.
type DiskAsyncReplicationProvisioner struct {
	*base.BaseResource
}

var _ prov.Provisioner = (*DiskAsyncReplicationProvisioner)(nil)

// asyncReplicationActiveState is the only state that counts as replicating.
const asyncReplicationActiveState = "ACTIVE"

func init() {
	registry.Register(DiskAsyncReplicationResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(cfg *config.Config) prov.Provisioner {
			return &DiskAsyncReplicationProvisioner{
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

// buildAsyncReplicationNativeID composes
// "projects/{p}/zones/{primaryZone}/disks/{primary}/asyncReplication/{secondaryZone}/{secondary}".
// Both ends have to survive: the verbs live on the primary, and only the
// secondary reports the state.
func buildAsyncReplicationNativeID(project, primaryZone, primary, secondaryZone, secondary string) string {
	return fmt.Sprintf("projects/%s/zones/%s/disks/%s/asyncReplication/%s/%s",
		project, primaryZone, primary, secondaryZone, secondary)
}

func parseAsyncReplicationNativeID(nativeID string) (project, primaryZone, primary, secondaryZone, secondary string, err error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 9 || parts[0] != "projects" || parts[2] != "zones" ||
		parts[4] != "disks" || parts[6] != "asyncReplication" ||
		parts[3] == "" || parts[5] == "" || parts[7] == "" || parts[8] == "" {
		return "", "", "", "", "", fmt.Errorf("invalid disk async replication native ID: %s", nativeID)
	}
	return parts[1], parts[3], parts[5], parts[7], parts[8], nil
}

// diskRefParts pulls the zone and name out of a disk reference, which may be a
// full URL or a bare "zones/{z}/disks/{n}" path.
func diskRefParts(ref string) (zone, name string, ok bool) {
	parts := strings.Split(strings.TrimSuffix(ref, "/"), "/")
	for i := len(parts) - 1; i >= 1; i-- {
		if parts[i-1] == "disks" {
			name = parts[i]
			break
		}
	}
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "zones" {
			zone = parts[i+1]
		}
	}
	return zone, name, zone != "" && name != ""
}

// asyncReplicationState reads the secondary disk's replication state, which is
// the only place the live/stopped distinction is reported.
func asyncReplicationState(secondaryDisk map[string]interface{}) string {
	status, ok := secondaryDisk["resourceStatus"].(map[string]interface{})
	if !ok {
		return ""
	}
	primary, ok := status["asyncPrimaryDisk"].(map[string]interface{})
	if !ok {
		return ""
	}
	state, _ := primary["state"].(string)
	return state
}

// asyncPrimaryDiskRef reads which primary the secondary is paired with.
func asyncPrimaryDiskRef(secondaryDisk map[string]interface{}) string {
	primary, ok := secondaryDisk["asyncPrimaryDisk"].(map[string]interface{})
	if !ok {
		return ""
	}
	disk, _ := primary["disk"].(string)
	return disk
}

func (p *DiskAsyncReplicationProvisioner) diskURL(project, zone, disk string) string {
	return fmt.Sprintf("%s/projects/%s/zones/%s/disks/%s",
		p.APIConfig.BaseURL, project, zone, disk)
}

func (p *DiskAsyncReplicationProvisioner) projectFor(targetConfig json.RawMessage, fallback string) string {
	if cfg := config.FromTargetConfig(targetConfig); cfg != nil && cfg.Project != "" {
		return cfg.Project
	}
	return fallback
}

func (p *DiskAsyncReplicationProvisioner) issueVerb(
	ctx context.Context, url string, body map[string]interface{}, project, zone string,
) (string, *transport.Error) {
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return "", transport.WrapError(err, "failed to create transport client")
	}
	resp, sErr := client.SendRequest(ctx, transport.RequestOptions{
		Method: "POST",
		URL:    url,
		Body:   body,
	})
	if sErr != nil {
		return "", transport.WrapError(sErr, "disk async replication verb failed")
	}
	opID := p.OperationConfig.OperationIDExtractor(resp.Body)
	return p.OperationConfig.OperationURLBuilder(
		base.PathContext{Project: project, Zone: zone}, opID), nil
}

func (p *DiskAsyncReplicationProvisioner) fetchDisk(
	ctx context.Context, project, zone, disk string,
) (map[string]interface{}, bool, *transport.Error) {
	client, cErr := transport.NewClient(ctx, p.Config)
	if cErr != nil {
		return nil, false, transport.WrapError(cErr, "failed to create transport client")
	}
	resp, rErr := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    p.diskURL(project, zone, disk),
	})
	if rErr != nil {
		wrapped := transport.WrapError(rErr, "failed to read disk")
		if transport.ToResourceErrorCode(wrapped.Code) == resource.OperationErrorCodeNotFound {
			return nil, true, nil
		}
		return nil, false, wrapped
	}
	return resp.Body, false, nil
}

func (p *DiskAsyncReplicationProvisioner) Create(
	ctx context.Context, request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid properties: %v", err)), nil
	}
	primaryRef, _ := props["primaryDisk"].(string)
	secondaryRef, _ := props["secondaryDisk"].(string)
	primaryZone, primary, okP := diskRefParts(primaryRef)
	secondaryZone, secondary, okS := diskRefParts(secondaryRef)
	if !okP || !okS {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"primaryDisk and secondaryDisk must be zonal disk references"), nil
	}
	project := p.projectFor(request.TargetConfig, "")
	if project == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"target project is required"), nil
	}

	requestID, verbErr := p.issueVerb(ctx,
		p.diskURL(project, primaryZone, primary)+"/startAsyncReplication",
		map[string]interface{}{"asyncSecondaryDisk": secondaryRef}, project, primaryZone)
	if verbErr != nil {
		return createFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID: buildAsyncReplicationNativeID(project, primaryZone, primary,
				secondaryZone, secondary),
			RequestID:     requestID,
			StatusMessage: "disk async replication start in progress",
		},
	}, nil
}

// Read judges by the secondary's replication state. Stopping replication leaves
// asyncPrimaryDisk in place, so presence of the field proves nothing.
func (p *DiskAsyncReplicationProvisioner) Read(
	ctx context.Context, request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	project, primaryZone, primary, secondaryZone, secondary, err :=
		parseAsyncReplicationNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}
	project = p.projectFor(request.TargetConfig, project)

	secondaryDisk, gone, fErr := p.fetchDisk(ctx, project, secondaryZone, secondary)
	if fErr != nil {
		return &resource.ReadResult{ErrorCode: transport.ToResourceErrorCode(fErr.Code)}, nil
	}
	if gone {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
	}
	if asyncReplicationState(secondaryDisk) != asyncReplicationActiveState {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
	}
	// Guard against the secondary having been re-paired with a different primary.
	if ref := asyncPrimaryDiskRef(secondaryDisk); ref != "" {
		zone, name, ok := diskRefParts(ref)
		if ok && (zone != primaryZone || name != primary) {
			return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
		}
	}

	encoded, mErr := json.Marshal(map[string]interface{}{
		"primaryDisk":   p.diskURL(project, primaryZone, primary),
		"secondaryDisk": p.diskURL(project, secondaryZone, secondary),
	})
	if mErr != nil {
		return nil, fmt.Errorf("failed to marshal async replication properties: %w", mErr)
	}
	return &resource.ReadResult{Properties: string(encoded)}, nil
}

func (p *DiskAsyncReplicationProvisioner) Update(
	ctx context.Context, request *resource.UpdateRequest,
) (*resource.UpdateResult, error) {
	return updateFailure(resource.OperationErrorCodeNotUpdatable,
		"disk async replication is a (primary, secondary) pair; a change replaces it"), nil
}

// Delete stops replication. The verb is idempotent, so stopping an
// already-stopped pair is not an error.
func (p *DiskAsyncReplicationProvisioner) Delete(
	ctx context.Context, request *resource.DeleteRequest,
) (*resource.DeleteResult, error) {
	project, primaryZone, primary, _, _, err := parseAsyncReplicationNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	project = p.projectFor(request.TargetConfig, project)

	requestID, verbErr := p.issueVerb(ctx,
		p.diskURL(project, primaryZone, primary)+"/stopAsyncReplication", nil, project, primaryZone)
	if verbErr != nil {
		// A primary that is already gone took the pairing with it.
		if transport.ToResourceErrorCode(verbErr.Code) == resource.OperationErrorCodeNotFound {
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusSuccess,
					NativeID:        request.NativeID,
					StatusMessage:   "primary disk already deleted",
				},
			}, nil
		}
		return deleteFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        request.NativeID,
			RequestID:       requestID,
			StatusMessage:   "disk async replication stop in progress",
		},
	}, nil
}

// List reports every active replication pair in the project. Discovery calls
// this with no hints, so it walks the aggregated disk list and reports each
// secondary whose replication is ACTIVE, naming the primary it is paired with.
// Stopped pairs are deliberately absent: Read treats them as gone, so listing
// them would produce ids that immediately read as not-found.
func (p *DiskAsyncReplicationProvisioner) List(
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

	nativeIDs := []string{}
	scopes, _ := resp.Body["items"].(map[string]interface{})
	for scope, payload := range scopes {
		if !strings.HasPrefix(scope, "zones/") {
			continue
		}
		secondaryZone := strings.TrimPrefix(scope, "zones/")
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
			if asyncReplicationState(disk) != asyncReplicationActiveState {
				continue
			}
			secondary, _ := disk["name"].(string)
			primaryZone, primary, ok := diskRefParts(asyncPrimaryDiskRef(disk))
			if !ok || secondary == "" {
				continue
			}
			nativeIDs = append(nativeIDs, buildAsyncReplicationNativeID(
				project, primaryZone, primary, secondaryZone, secondary))
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// Status routes through the shared read-back so post-create state reflects the
// live pairing rather than only what was declared.
func (p *DiskAsyncReplicationProvisioner) Status(
	ctx context.Context, request *resource.StatusRequest,
) (*resource.StatusResult, error) {
	return base.StatusWithRead(ctx, p.BaseResource, p.Read, request)
}
