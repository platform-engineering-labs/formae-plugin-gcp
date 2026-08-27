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

// ProjectMetadataItemProvisioner manages one key of a project's common instance
// metadata — the project-wide defaults every VM inherits, such as
// enable-oslogin, ssh-keys or a default startup-script.
//
// The API has no per-key operation: setCommonInstanceMetadata replaces the whole
// list. So every write here is read-modify-write, touching one key and passing
// every other key through untouched. A stale fingerprint is rejected by the API
// rather than silently overwriting, which is the safety net that makes this
// approach sound when something else edits metadata concurrently.
type ProjectMetadataItemProvisioner struct {
	*base.BaseResource
}

var _ prov.Provisioner = (*ProjectMetadataItemProvisioner)(nil)

func init() {
	registry.Register(ProjectMetadataItemResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(cfg *config.Config) prov.Provisioner {
			return &ProjectMetadataItemProvisioner{
				BaseResource: &base.BaseResource{
					Config:          cfg,
					APIConfig:       ComputeAPI,
					OperationConfig: ComputeOperations,
					ResourceConfig: base.ResourceConfig{
						ResourceType: "projects",
						Scope:        &base.ScopeConfig{Type: base.ScopeGlobal},
					},
					NativeIDConfig: ComputeNativeID,
				},
			}
		})
}

// buildMetadataItemNativeID composes
// "projects/{p}/commonInstanceMetadata/items/{key}".
func buildMetadataItemNativeID(project, key string) string {
	return fmt.Sprintf("projects/%s/commonInstanceMetadata/items/%s", project, key)
}

func parseMetadataItemNativeID(nativeID string) (project, key string, err error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 5 || parts[0] != "projects" || parts[2] != "commonInstanceMetadata" ||
		parts[3] != "items" || parts[4] == "" {
		return "", "", fmt.Errorf("invalid project metadata item native ID: %s", nativeID)
	}
	return parts[1], parts[4], nil
}

// mergeMetadataItem returns the item list with one key set to a value, leaving
// every other key alone and preserving their order. Passing an empty value with
// remove=true deletes the key instead.
//
// This is the whole safety story of this resource: setCommonInstanceMetadata
// replaces the list wholesale, so anything dropped here is dropped from the
// project.
func mergeMetadataItem(items []interface{}, key, value string, remove bool) []map[string]interface{} {
	merged := make([]map[string]interface{}, 0, len(items)+1)
	found := false
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := item["key"].(string)
		if name == "" {
			continue
		}
		if name == key {
			found = true
			if remove {
				continue
			}
			merged = append(merged, map[string]interface{}{"key": key, "value": value})
			continue
		}
		// Copy foreign keys verbatim - they belong to someone else.
		copied := map[string]interface{}{"key": name}
		if v, ok := item["value"].(string); ok {
			copied["value"] = v
		}
		merged = append(merged, copied)
	}
	if !found && !remove {
		merged = append(merged, map[string]interface{}{"key": key, "value": value})
	}
	return merged
}

// metadataItemValue finds one key in a metadata item list.
func metadataItemValue(items []interface{}, key string) (string, bool) {
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if name, _ := item["key"].(string); name == key {
			value, _ := item["value"].(string)
			return value, true
		}
	}
	return "", false
}

func (p *ProjectMetadataItemProvisioner) projectFor(targetConfig json.RawMessage, fallback string) string {
	if cfg := config.FromTargetConfig(targetConfig, nil /* path context only; this config never authenticates */); cfg != nil && cfg.Project != "" {
		return cfg.Project
	}
	return fallback
}

func (p *ProjectMetadataItemProvisioner) projectURL(project string) string {
	return fmt.Sprintf("%s/projects/%s", p.APIConfig.BaseURL, project)
}

// readMetadata fetches the project's common instance metadata, returning the
// current fingerprint alongside the items.
func (p *ProjectMetadataItemProvisioner) readMetadata(
	ctx context.Context, project string,
) (items []interface{}, fingerprint string, err *transport.Error) {
	client, cErr := transport.NewClient(ctx, p.Config)
	if cErr != nil {
		return nil, "", transport.WrapError(cErr, "failed to create transport client")
	}
	resp, rErr := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    p.projectURL(project),
	})
	if rErr != nil {
		return nil, "", transport.WrapError(rErr, "failed to read project metadata")
	}
	metadata, _ := resp.Body["commonInstanceMetadata"].(map[string]interface{})
	if metadata == nil {
		return nil, "", nil
	}
	fingerprint, _ = metadata["fingerprint"].(string)
	items, _ = metadata["items"].([]interface{})
	return items, fingerprint, nil
}

// writeMetadataItem applies one key change. It re-reads on a fingerprint
// conflict, which happens when something else edited metadata in between; a
// single retry is enough for the racing writer to have finished, and the API
// rejects rather than clobbers, so no foreign key can be lost either way.
func (p *ProjectMetadataItemProvisioner) writeMetadataItem(
	ctx context.Context, project, key, value string, remove bool,
) (string, *transport.Error) {
	client, cErr := transport.NewClient(ctx, p.Config)
	if cErr != nil {
		return "", transport.WrapError(cErr, "failed to create transport client")
	}

	var lastErr *transport.Error
	for attempt := 0; attempt < 2; attempt++ {
		items, fingerprint, rErr := p.readMetadata(ctx, project)
		if rErr != nil {
			return "", rErr
		}
		body := map[string]interface{}{
			"fingerprint": fingerprint,
			"items":       mergeMetadataItem(items, key, value, remove),
		}
		resp, sErr := client.SendRequest(ctx, transport.RequestOptions{
			Method: "POST",
			URL:    p.projectURL(project) + "/setCommonInstanceMetadata",
			Body:   body,
		})
		if sErr == nil {
			opID := p.OperationConfig.OperationIDExtractor(resp.Body)
			return p.OperationConfig.OperationURLBuilder(
				base.PathContext{Project: project}, opID), nil
		}
		lastErr = transport.WrapError(sErr, "failed to set project metadata")
		if !strings.Contains(strings.ToLower(lastErr.Message), "fingerprint") {
			break
		}
	}
	return "", lastErr
}

func (p *ProjectMetadataItemProvisioner) Create(
	ctx context.Context, request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid properties: %v", err)), nil
	}
	key, _ := props["key"].(string)
	value, _ := props["value"].(string)
	if key == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest, "key is required"), nil
	}
	project := p.projectFor(request.TargetConfig, "")
	if project == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"target project is required"), nil
	}

	requestID, verbErr := p.writeMetadataItem(ctx, project, key, value, false)
	if verbErr != nil {
		return createFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        buildMetadataItemNativeID(project, key),
			RequestID:       requestID,
			StatusMessage:   "project metadata item creation in progress",
		},
	}, nil
}

func (p *ProjectMetadataItemProvisioner) Read(
	ctx context.Context, request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	project, key, err := parseMetadataItemNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}
	project = p.projectFor(request.TargetConfig, project)

	items, _, rErr := p.readMetadata(ctx, project)
	if rErr != nil {
		return &resource.ReadResult{ErrorCode: transport.ToResourceErrorCode(rErr.Code)}, nil
	}
	value, found := metadataItemValue(items, key)
	if !found {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
	}

	encoded, mErr := json.Marshal(map[string]interface{}{"key": key, "value": value})
	if mErr != nil {
		return nil, fmt.Errorf("failed to marshal metadata item properties: %w", mErr)
	}
	return &resource.ReadResult{Properties: string(encoded)}, nil
}

func (p *ProjectMetadataItemProvisioner) Update(
	ctx context.Context, request *resource.UpdateRequest,
) (*resource.UpdateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.DesiredProperties, &props); err != nil {
		return updateFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid properties: %v", err)), nil
	}
	project, key, err := parseMetadataItemNativeID(request.NativeID)
	if err != nil {
		return updateFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	project = p.projectFor(request.TargetConfig, project)
	value, _ := props["value"].(string)

	requestID, verbErr := p.writeMetadataItem(ctx, project, key, value, false)
	if verbErr != nil {
		return updateFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        request.NativeID,
			RequestID:       requestID,
			StatusMessage:   "project metadata item update in progress",
		},
	}, nil
}

func (p *ProjectMetadataItemProvisioner) Delete(
	ctx context.Context, request *resource.DeleteRequest,
) (*resource.DeleteResult, error) {
	project, key, err := parseMetadataItemNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	project = p.projectFor(request.TargetConfig, project)

	requestID, verbErr := p.writeMetadataItem(ctx, project, key, "", true)
	if verbErr != nil {
		return deleteFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        request.NativeID,
			RequestID:       requestID,
			StatusMessage:   "project metadata item deletion in progress",
		},
	}, nil
}

// List reports every key the project carries. Keys nobody declared show up as
// unmanaged, which is honest: they really are project-wide settings.
func (p *ProjectMetadataItemProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	project := p.projectFor(request.TargetConfig, "")
	if project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}
	items, _, rErr := p.readMetadata(ctx, project)
	if rErr != nil {
		return nil, fmt.Errorf("%s", rErr.Message)
	}
	nativeIDs := make([]string, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if key, _ := item["key"].(string); key != "" {
			nativeIDs = append(nativeIDs, buildMetadataItemNativeID(project, key))
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// Status routes through the shared read-back so post-create and post-update
// state carries the resource's real properties, not just what was declared.
func (p *ProjectMetadataItemProvisioner) Status(
	ctx context.Context, request *resource.StatusRequest,
) (*resource.StatusResult, error) {
	return base.StatusWithRead(ctx, p.BaseResource, p.Read, request)
}
