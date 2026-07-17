// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// InstanceGroupProvisioner manages an unmanaged zonal instance group and its VM
// membership. The group object + namedPorts are handled by the base resource;
// membership is verb-style (addInstances / removeInstances / listInstances) and
// namedPorts mutation needs setNamedPorts with a fingerprint, so Create, Read
// and Update are overridden. Delete/List/Status delegate to the base.
//
// Shaped like DiskProvisioner (delegate what the base handles, override the
// verb-driven bits).
type InstanceGroupProvisioner struct {
	*base.BaseResource
}

var _ prov.Provisioner = (*InstanceGroupProvisioner)(nil)

func init() {
	registry.Register(InstanceGroupResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(cfg *config.Config) prov.Provisioner {
			return newInstanceGroupProvisioner(cfg)
		})
}

func newInstanceGroupProvisioner(cfg *config.Config) *InstanceGroupProvisioner {
	return &InstanceGroupProvisioner{
		BaseResource: &base.BaseResource{
			Config:          cfg,
			APIConfig:       ComputeAPI,
			OperationConfig: ComputeOperations,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "instanceGroups",
				Scope:          &base.ScopeConfig{Type: base.ScopeZonal},
				SupportsUpdate: true, // membership + namedPorts, via the custom Update
			},
			NativeIDConfig: ComputeNativeID,
			// insert rejects a members payload; strip it and attach after create.
			RequestTransformer:  base.RequestTransformerFunc(stripInstancesForInsert),
			ResponseTransformer: base.ZoneResponseTransformer,
		},
	}
}

// ---------------------------------------------------------------------------
// Pure helpers (unit-tested in instance_group_test.go)
// ---------------------------------------------------------------------------

// rawPageMembers extracts items[].instance URLs from one listInstances page,
// in API order.
func rawPageMembers(body map[string]interface{}) []string {
	items := utils.GetArray(body, "items")
	out := make([]string, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if url := utils.GetString(item, "instance"); url != "" {
			out = append(out, url)
		}
	}
	return out
}

// membersFromListInstancesResponse returns a single page's members, sorted.
func membersFromListInstancesResponse(body map[string]interface{}) []string {
	m := rawPageMembers(body)
	sort.Strings(m)
	return m
}

// diffMembers computes the add/remove sets between desired and current
// membership (both full self-link URLs). Returns nil (not empty slices) when a
// direction needs no change, so callers can `if len(x) > 0` and tests compare
// cleanly. Order-independent.
func diffMembers(desired, current []string) (toAdd, toRemove []string) {
	cur := make(map[string]struct{}, len(current))
	for _, c := range current {
		cur[c] = struct{}{}
	}
	des := make(map[string]struct{}, len(desired))
	for _, d := range desired {
		des[d] = struct{}{}
	}
	for _, d := range desired {
		if _, ok := cur[d]; !ok {
			toAdd = append(toAdd, d)
		}
	}
	for _, c := range current {
		if _, ok := des[c]; !ok {
			toRemove = append(toRemove, c)
		}
	}
	sort.Strings(toAdd)
	sort.Strings(toRemove)
	return toAdd, toRemove
}

// instancesVerbBody builds the shared {"instances":[{"instance":url}]} payload
// for addInstances and removeInstances.
func instancesVerbBody(urls []string) map[string]interface{} {
	list := make([]map[string]interface{}, 0, len(urls))
	for _, u := range urls {
		list = append(list, map[string]interface{}{"instance": u})
	}
	return map[string]interface{}{"instances": list}
}

// setNamedPortsBody builds the setNamedPorts payload; the fingerprint is the
// group's current optimistic-lock token.
func setNamedPortsBody(namedPorts []interface{}, fingerprint string) map[string]interface{} {
	return map[string]interface{}{
		"namedPorts":  namedPorts,
		"fingerprint": fingerprint,
	}
}

// desiredMembers extracts the desired VM self-links from a forma's instances
// field, sorted. Absent field → nil.
func desiredMembers(props map[string]interface{}) []string {
	arr := utils.GetArray(props, "instances")
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

// mergeMembershipIntoProps writes live membership into read-back props: sorted
// members when present, field omitted when the group is empty so it equals an
// absent desired field (mirrors how normalizeInstanceDisks omits noise).
func mergeMembershipIntoProps(props map[string]interface{}, members []string) {
	if len(members) == 0 {
		delete(props, "instances")
		return
	}
	sorted := append([]string(nil), members...)
	sort.Strings(sorted)
	props["instances"] = sorted
}

// stripInstancesForInsert removes members from the insert body; membership is
// attached after the group exists (cf. sslCertificateRequestTransformer).
func stripInstancesForInsert(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		if k == "instances" {
			continue
		}
		body[k] = v
	}
	return body, nil
}

// namedPortsEqual compares two namedPorts lists as name→port sets.
func namedPortsEqual(a, b []interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	return reflect.DeepEqual(namedPortMap(a), namedPortMap(b))
}

func namedPortMap(ports []interface{}) map[string]interface{} {
	m := make(map[string]interface{}, len(ports))
	for _, raw := range ports {
		p, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		m[utils.GetString(p, "name")] = p["port"]
	}
	return m
}

// ---------------------------------------------------------------------------
// Provisioner methods
// ---------------------------------------------------------------------------

// groupURL builds the group's resource URL from a native ID + target config.
func (p *InstanceGroupProvisioner) groupURL(targetConfig json.RawMessage, nativeID string) (string, base.PathContext, error) {
	pathCtx, err := base.ParseNativeID(p.NativeIDConfig, nativeID)
	if err != nil {
		return "", base.PathContext{}, err
	}
	p.FillPathContextFromTarget(targetConfig, &pathCtx)
	pathCtx.ResourceType = p.ResourceConfig.ResourceType
	urlBuilder := base.NewURLBuilder(p.APIConfig, pathCtx)
	return urlBuilder.ResourceURL(pathCtx.ResourceName), pathCtx, nil
}

// issueVerb POSTs a membership/namedPorts verb and returns the operation path.
func (p *InstanceGroupProvisioner) issueVerb(
	ctx context.Context, client *transport.Client,
	verbURL string, body map[string]interface{}, pathCtx base.PathContext,
) (string, error) {
	resp, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "POST",
		URL:    verbURL,
		Body:   body,
	})
	if err != nil {
		wrapped := transport.WrapError(err, "instance group verb failed")
		return "", fmt.Errorf("%s", wrapped.Message)
	}
	opID := p.OperationConfig.OperationIDExtractor(resp.Body)
	return p.OperationConfig.OperationURLBuilder(pathCtx, opID), nil
}

// waitOp blocks until the given compute operation completes.
func (p *InstanceGroupProvisioner) waitOp(ctx context.Context, client *transport.Client, opPath string) error {
	waiter := transport.NewOperationWaiter(client, opPath, fmt.Sprintf("%s/%s", p.APIConfig.BaseURL, opPath))
	result, err := waiter.Wait(ctx)
	if err != nil {
		return err
	}
	if result.Status != transport.OperationStatusSuccess {
		return fmt.Errorf("operation %s: %s", opPath, result.Message)
	}
	return nil
}

// listMembers reads live membership via paginated listInstances.
func (p *InstanceGroupProvisioner) listMembers(ctx context.Context, client *transport.Client, groupURL string) ([]string, error) {
	var all []string
	pageToken := ""
	for {
		url := groupURL + "/listInstances"
		if pageToken != "" {
			var err error
			if url, err = transport.AddQueryParam(url, "pageToken", pageToken); err != nil {
				return nil, err
			}
		}
		resp, err := client.SendRequest(ctx, transport.RequestOptions{
			Method: "POST",
			URL:    url,
			Body:   map[string]interface{}{"instanceState": "ALL"},
		})
		if err != nil {
			wrapped := transport.WrapError(err, "failed to list instance group members")
			// An empty group can 404 on listInstances in some states; treat as no members.
			if transport.ToResourceErrorCode(wrapped.Code) == resource.OperationErrorCodeNotFound {
				break
			}
			return nil, fmt.Errorf("%s", wrapped.Message)
		}
		all = append(all, rawPageMembers(resp.Body)...)
		pageToken = utils.GetString(resp.Body, "nextPageToken")
		if pageToken == "" {
			break
		}
	}
	sort.Strings(all)
	return all, nil
}

// Read merges live membership into the base read.
func (p *InstanceGroupProvisioner) Read(ctx context.Context, request *resource.ReadRequest) (*resource.ReadResult, error) {
	res, err := p.BaseResource.Read(ctx, request)
	if err != nil || res.ErrorCode != "" {
		return res, err
	}

	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}
	groupURL, _, err := p.groupURL(request.TargetConfig, request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}

	members, err := p.listMembers(ctx, client, groupURL)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeServiceInternalError}, nil
	}

	var props map[string]interface{}
	if err := json.Unmarshal([]byte(res.Properties), &props); err != nil {
		return nil, fmt.Errorf("failed to parse base read properties: %w", err)
	}
	mergeMembershipIntoProps(props, members)

	propsJSON, err := json.Marshal(props)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal properties: %w", err)
	}
	return &resource.ReadResult{Properties: string(propsJSON)}, nil
}

// Create inserts the group (members stripped by the request transformer), waits
// for it to exist, then attaches the desired members.
func (p *InstanceGroupProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid properties: %v", err)), nil
	}
	members := desiredMembers(props)

	res, err := p.BaseResource.Create(ctx, request)
	if err != nil {
		return nil, err
	}
	if res.ProgressResult == nil ||
		res.ProgressResult.OperationStatus == resource.OperationStatusFailure ||
		len(members) == 0 {
		return res, nil
	}

	// Attach members once the group exists. Do it synchronously so the group is
	// non-empty by the time formae polls Status.
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}
	if err := p.waitOp(ctx, client, res.ProgressResult.RequestID); err != nil {
		return createFailure(resource.OperationErrorCodeServiceInternalError,
			fmt.Sprintf("group insert did not complete: %v", err)), nil
	}

	groupURL, pathCtx, err := p.groupURL(request.TargetConfig, res.ProgressResult.NativeID)
	if err != nil {
		return createFailure(resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}
	opPath, err := p.issueVerb(ctx, client, groupURL+"/addInstances", instancesVerbBody(members), pathCtx)
	if err != nil {
		return createFailure(resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        res.ProgressResult.NativeID,
			RequestID:       opPath,
			StatusMessage:   "instance group member attach in progress",
		},
	}, nil
}

// Update reconciles membership (addInstances/removeInstances) and namedPorts
// (setNamedPorts). Intermediate operations are awaited synchronously; the final
// one is returned InProgress so base Status reports it and reads back.
func (p *InstanceGroupProvisioner) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	var desired map[string]interface{}
	if err := json.Unmarshal(request.DesiredProperties, &desired); err != nil {
		return updateFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid desired properties: %v", err)), nil
	}

	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}
	groupURL, pathCtx, err := p.groupURL(request.TargetConfig, request.NativeID)
	if err != nil {
		return updateFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	// Current group state: GET for fingerprint + namedPorts, listInstances for members.
	getResp, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: groupURL})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to read instance group")
		return updateFailure(transport.ToResourceErrorCode(wrapped.Code), wrapped.Message), nil
	}
	currentMembers, err := p.listMembers(ctx, client, groupURL)
	if err != nil {
		return updateFailure(resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}

	toAdd, toRemove := diffMembers(desiredMembers(desired), currentMembers)

	// Build the ordered list of verbs to issue.
	type verb struct {
		url  string
		body map[string]interface{}
	}
	var verbs []verb
	if len(toAdd) > 0 {
		verbs = append(verbs, verb{groupURL + "/addInstances", instancesVerbBody(toAdd)})
	}
	if len(toRemove) > 0 {
		verbs = append(verbs, verb{groupURL + "/removeInstances", instancesVerbBody(toRemove)})
	}
	desiredPorts := utils.GetArray(desired, "namedPorts")
	currentPorts := utils.GetArray(getResp.Body, "namedPorts")
	if !namedPortsEqual(desiredPorts, currentPorts) {
		fingerprint := utils.GetString(getResp.Body, "fingerprint")
		verbs = append(verbs, verb{groupURL + "/setNamedPorts", setNamedPortsBody(desiredPorts, fingerprint)})
	}

	if len(verbs) == 0 {
		// Nothing to do — report success with a fresh read.
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusSuccess,
				NativeID:        request.NativeID,
				StatusMessage:   "instance group already reconciled",
			},
		}, nil
	}

	var lastOp string
	for i, v := range verbs {
		opPath, err := p.issueVerb(ctx, client, v.url, v.body, pathCtx)
		if err != nil {
			return updateFailure(resource.OperationErrorCodeServiceInternalError, err.Error()), nil
		}
		// Await intermediate operations so verbs apply in order; leave the last
		// one for formae's Status poll.
		if i < len(verbs)-1 {
			if err := p.waitOp(ctx, client, opPath); err != nil {
				return updateFailure(resource.OperationErrorCodeServiceInternalError, err.Error()), nil
			}
		}
		lastOp = opPath
	}

	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        request.NativeID,
			RequestID:       lastOp,
			StatusMessage:   "instance group reconcile in progress",
		},
	}, nil
}

// Delete delegates to the base — deleting a group with members is legal, no
// detach needed.
func (p *InstanceGroupProvisioner) Delete(ctx context.Context, request *resource.DeleteRequest) (*resource.DeleteResult, error) {
	return p.BaseResource.Delete(ctx, request)
}

// List delegates to the base.
func (p *InstanceGroupProvisioner) List(ctx context.Context, request *resource.ListRequest) (*resource.ListResult, error) {
	return p.BaseResource.List(ctx, request)
}

// Status delegates to the base and reads back on success (like DiskProvisioner),
// so the reported properties include reconciled membership.
func (p *InstanceGroupProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	result, err := p.BaseResource.Status(ctx, request)
	if err != nil {
		return nil, err
	}
	if result.ProgressResult.OperationStatus == resource.OperationStatusSuccess &&
		result.ProgressResult.NativeID != "" {
		readResult, err := p.Read(ctx, &resource.ReadRequest{
			NativeID:     result.ProgressResult.NativeID,
			ResourceType: request.ResourceType,
			TargetConfig: request.TargetConfig,
		})
		if err == nil && readResult.ErrorCode == "" {
			result.ProgressResult.ResourceProperties = []byte(readResult.Properties)
		}
	}
	return result, nil
}
