// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

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

// stripProviderEchoedFields drops network/subnetwork from a read-back. They are
// createOnly resolvable scalars: formae extracts resolvable scalars out of the
// forma's rendered Properties, so they are never in the desired/expected set,
// yet GCP echoes them on instanceGroups.get once a member is attached (it
// derives them from the member's NIC). Echoing them back reads as perpetual
// "not expected" drift, so they are stripped — mirroring RouterNat dropping
// provider-populated fields. A forma still sets them on create (needed so the
// group lands on the member's network); they just do not round-trip on read.
func stripProviderEchoedFields(props map[string]interface{}) {
	delete(props, "network")
	delete(props, "subnetwork")
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

// igmDelim separates the pending operation path from the base64 reconcile plan
// in a RequestID. Membership + named-port verbs are async and there can be
// several per reconcile, so — rather than blocking the Create/Update RPC — the
// desired state rides in the RequestID and Status drives one verb per poll,
// re-deriving the remaining work from live state each time (idempotent,
// restart-safe). Mirrors RouterNat encoding its NAT id in the RequestID.
const igmDelim = "|igm="

type reconcilePlan struct {
	M  []string      `json:"m"`
	NP []interface{} `json:"np"`
}

// encodeReconcile packs the pending op path + desired plan into a RequestID.
func encodeReconcile(opPath string, members []string, namedPorts []interface{}) string {
	b, _ := json.Marshal(reconcilePlan{M: members, NP: namedPorts})
	return opPath + igmDelim + base64.RawURLEncoding.EncodeToString(b)
}

// decodeReconcile is the inverse; ok=false means the RequestID carries no plan
// (a bare op path), so callers fall back to a plain operation poll.
func decodeReconcile(requestID string) (opPath string, members []string, namedPorts []interface{}, ok bool) {
	i := strings.Index(requestID, igmDelim)
	if i < 0 {
		return requestID, nil, nil, false
	}
	opPath = requestID[:i]
	raw, err := base64.RawURLEncoding.DecodeString(requestID[i+len(igmDelim):])
	if err != nil {
		return opPath, nil, nil, false
	}
	var p reconcilePlan
	if err := json.Unmarshal(raw, &p); err != nil {
		return opPath, nil, nil, false
	}
	return opPath, p.M, p.NP, true
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
	stripProviderEchoedFields(props)
	mergeMembershipIntoProps(props, members)

	propsJSON, err := json.Marshal(props)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal properties: %w", err)
	}
	return &resource.ReadResult{Properties: string(propsJSON)}, nil
}

// Create inserts the group (members stripped by the request transformer;
// namedPorts ride along on the insert body) and returns InProgress immediately.
// The insert op path plus the desired member set are packed into the RequestID
// so Status attaches members once the group exists — the Create RPC must return
// fast (blocking it past the operator deadline fails the create).
func (p *InstanceGroupProvisioner) Create(ctx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid properties: %v", err)), nil
	}

	res, err := p.BaseResource.Create(ctx, request)
	if err != nil {
		return nil, err
	}
	if res.ProgressResult == nil || res.ProgressResult.OperationStatus == resource.OperationStatusFailure {
		return res, nil
	}

	// Pack the desired plan onto the insert op path; Status reconciles membership
	// (and namedPorts, defensively) after the insert completes.
	res.ProgressResult.RequestID = encodeReconcile(
		res.ProgressResult.RequestID, desiredMembers(props), utils.GetArray(props, "namedPorts"))
	res.ProgressResult.StatusMessage = "instance group creation in progress"
	return res, nil
}

// Update returns InProgress immediately with the desired plan encoded and no
// pending op (empty op path); Status does the whole membership + namedPorts
// reconcile, one verb per poll, so the Update RPC never blocks.
func (p *InstanceGroupProvisioner) Update(ctx context.Context, request *resource.UpdateRequest) (*resource.UpdateResult, error) {
	var desired map[string]interface{}
	if err := json.Unmarshal(request.DesiredProperties, &desired); err != nil {
		return updateFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid desired properties: %v", err)), nil
	}

	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        request.NativeID,
			RequestID:       encodeReconcile("", desiredMembers(desired), utils.GetArray(desired, "namedPorts")),
			StatusMessage:   "instance group reconcile pending",
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

// Status is the reconcile engine. It (1) waits for any pending compute
// operation encoded in the RequestID, then (2) re-derives the remaining work
// from live state and issues at most one verb (addInstances, then
// removeInstances, then setNamedPorts), returning InProgress until the group
// matches the desired plan — at which point it reads back and reports Success.
// Re-deriving from live state each poll makes it idempotent and restart-safe.
func (p *InstanceGroupProvisioner) Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error) {
	opPath, members, namedPorts, ok := decodeReconcile(request.RequestID)
	if !ok {
		// No reconcile plan (shouldn't happen from our Create/Update) — fall back
		// to a plain operation poll so we never silently drop membership.
		return p.BaseResource.Status(ctx, request)
	}

	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}
	groupURL, pathCtx, err := p.groupURL(request.TargetConfig, request.NativeID)
	if err != nil {
		return p.statusFailure(request, resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	// (1) Wait for the pending operation, if any.
	if opPath != "" {
		resp, err := client.SendRequest(ctx, transport.RequestOptions{
			Method: "GET", URL: fmt.Sprintf("%s/%s", p.APIConfig.BaseURL, opPath),
		})
		if err != nil {
			wrapped := transport.WrapError(err, "failed to get operation status")
			return p.statusFailure(request, transport.ToResourceErrorCode(wrapped.Code), wrapped.Message), nil
		}
		done, checkErr := p.OperationConfig.OperationStatusChecker(resp.Body)
		if checkErr != nil {
			return p.statusFailure(request, resource.OperationErrorCodeServiceInternalError, checkErr.Error()), nil
		}
		if !done {
			return p.statusInProgress(request, "operation in progress"), nil
		}
	}

	// (2) Re-derive remaining work from live state and issue at most one verb.
	current, err := p.listMembers(ctx, client, groupURL)
	if err != nil {
		return p.statusFailure(request, resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}
	getResp, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: groupURL})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to read instance group")
		return p.statusFailure(request, transport.ToResourceErrorCode(wrapped.Code), wrapped.Message), nil
	}
	toAdd, toRemove := diffMembers(members, current)

	var verbURL string
	var body map[string]interface{}
	switch {
	case len(toAdd) > 0:
		verbURL, body = groupURL+"/addInstances", instancesVerbBody(toAdd)
	case len(toRemove) > 0:
		verbURL, body = groupURL+"/removeInstances", instancesVerbBody(toRemove)
	case !namedPortsEqual(namedPorts, utils.GetArray(getResp.Body, "namedPorts")):
		verbURL = groupURL + "/setNamedPorts"
		body = setNamedPortsBody(namedPorts, utils.GetString(getResp.Body, "fingerprint"))
	default:
		// Fully reconciled — read back and report success.
		return p.statusSuccess(ctx, request), nil
	}

	newOp, err := p.issueVerb(ctx, client, verbURL, body, pathCtx)
	if err != nil {
		return p.statusFailure(request, resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        request.NativeID,
			RequestID:       encodeReconcile(newOp, members, namedPorts),
			StatusMessage:   "instance group reconcile in progress",
		},
	}, nil
}

func (p *InstanceGroupProvisioner) statusInProgress(request *resource.StatusRequest, msg string) *resource.StatusResult {
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        request.NativeID,
			RequestID:       request.RequestID,
			StatusMessage:   msg,
		},
	}
}

func (p *InstanceGroupProvisioner) statusFailure(request *resource.StatusRequest, code resource.OperationErrorCode, msg string) *resource.StatusResult {
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       code,
			StatusMessage:   msg,
			NativeID:        request.NativeID,
			RequestID:       request.RequestID,
		},
	}
}

func (p *InstanceGroupProvisioner) statusSuccess(ctx context.Context, request *resource.StatusRequest) *resource.StatusResult {
	result := &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:        request.NativeID,
			StatusMessage:   "instance group reconcile completed",
		},
	}
	readResult, err := p.Read(ctx, &resource.ReadRequest{
		NativeID:     request.NativeID,
		ResourceType: request.ResourceType,
		TargetConfig: request.TargetConfig,
	})
	if err == nil && readResult.ErrorCode == "" {
		result.ProgressResult.ResourceProperties = []byte(readResult.Properties)
	}
	return result
}
