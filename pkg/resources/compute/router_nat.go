// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// RouterNatProvisioner handles CRUD for GCP::Compute::RouterNat.
//
// Cloud NAT isn't a standalone REST resource; it lives in Router.nats[] and is
// managed by read-modify-write on the parent via routers.patch. Sibling NATs
// on the same router survive every operation.
type RouterNatProvisioner struct {
	*base.BaseResource
}

var _ prov.Provisioner = (*RouterNatProvisioner)(nil)

// NewRouterNatProvisioner builds the provisioner.
func NewRouterNatProvisioner(cfg *config.Config) prov.Provisioner {
	return &RouterNatProvisioner{
		BaseResource: &base.BaseResource{
			Config:          cfg,
			APIConfig:       ComputeAPI,
			OperationConfig: ComputeOperations,
			// Parent type used for URL building only — NAT has no
			// standalone REST type.
			ResourceConfig: base.ResourceConfig{
				ResourceType: "routers",
				Scope:        &base.ScopeConfig{Type: base.ScopeRegional},
			},
			NativeIDConfig: ComputeNativeID,
		},
	}
}

// NAT NativeIDs look like "projects/{p}/regions/{r}/routers/{router}/nats/{nat}".
var natNativeIDRe = regexp.MustCompile(
	`^projects/([^/]+)/regions/([^/]+)/routers/([^/]+)/nats/([^/]+)$`,
)

// Status needs both the regional compute operation path and the NAT's
// synthetic NativeID; we stash both in RequestID with this delimiter so the
// operation poller knows which NAT it's reporting on.
const natRequestIDDelim = "|nat="

func encodeNatRequestID(opPath, natID string) string {
	return opPath + natRequestIDDelim + natID
}

func parseNatRequestID(requestID string) (opPath, natID string) {
	if i := strings.Index(requestID, natRequestIDDelim); i >= 0 {
		return requestID[:i], requestID[i+len(natRequestIDDelim):]
	}
	return requestID, ""
}

func parseNatNativeID(id string) (project, region, router, nat string, err error) {
	m := natNativeIDRe.FindStringSubmatch(id)
	if m == nil {
		return "", "", "", "", fmt.Errorf("invalid RouterNat NativeID: %s", id)
	}
	return m[1], m[2], m[3], m[4], nil
}

func buildNatNativeID(project, region, router, nat string) string {
	return fmt.Sprintf("projects/%s/regions/%s/routers/%s/nats/%s",
		project, region, router, nat)
}

func (p *RouterNatProvisioner) routerURL(project, region, router string) string {
	return fmt.Sprintf("%s/projects/%s/regions/%s/routers/%s",
		p.APIConfig.BaseURL, project, region, router)
}

// routerRefName accepts either a bare name or a Compute selfLink and returns
// the last path segment.
func routerRefName(v interface{}) string {
	s, _ := v.(string)
	if s == "" {
		return ""
	}
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// formae-level fields that don't belong on the wire to GCP.
var formaeOnlyProps = map[string]struct{}{
	"router": {}, "region": {}, "target": {}, "stack": {}, "label": {},
}

func natBodyFromProps(props map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(props))
	for k, v := range props {
		if _, drop := formaeOnlyProps[k]; drop {
			continue
		}
		out[k] = v
	}
	return out
}

func findNat(nats []interface{}, name string) int {
	for i, raw := range nats {
		n, _ := raw.(map[string]interface{})
		if utils.GetString(n, "name") == name {
			return i
		}
	}
	return -1
}

func natsOf(router map[string]interface{}) []interface{} {
	if router == nil {
		return nil
	}
	v, _ := router["nats"].([]interface{})
	return v
}

// fetchRouter returns (nil, nil) when the parent router is gone.
func (p *RouterNatProvisioner) fetchRouter(
	ctx context.Context, client *transport.Client,
	project, region, router string,
) (map[string]interface{}, error) {
	resp, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    p.routerURL(project, region, router),
	})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to read router")
		if transport.ToResourceErrorCode(wrapped.Code) == resource.OperationErrorCodeNotFound {
			return nil, nil
		}
		return nil, errors.New(wrapped.Message)
	}
	return resp.Body, nil
}

func (p *RouterNatProvisioner) patchNats(
	ctx context.Context, client *transport.Client,
	project, region, router string, nats []interface{},
) (*transport.Response, error) {
	return client.SendRequest(ctx, transport.RequestOptions{
		Method: "PATCH",
		URL:    p.routerURL(project, region, router),
		Body:   map[string]interface{}{"nats": nats},
	})
}

func (p *RouterNatProvisioner) Create(
	ctx context.Context, request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid properties: %v", err)), nil
	}

	name := utils.GetString(props, "name")
	region := utils.GetString(props, "region")
	router := routerRefName(props["router"])
	if name == "" || region == "" || router == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"RouterNat requires non-empty name, region, and router"), nil
	}

	cfg := config.FromTargetConfig(request.TargetConfig)
	parent, err := p.fetchRouter(ctx, client, cfg.Project, region, router)
	if err != nil {
		return createFailure(resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}
	if parent == nil {
		return createFailure(resource.OperationErrorCodeNotFound,
			fmt.Sprintf("parent router %s not found in region %s", router, region)), nil
	}

	nats := natsOf(parent)
	if findNat(nats, name) >= 0 {
		return createFailure(resource.OperationErrorCodeAlreadyExists,
			fmt.Sprintf("RouterNat %s already exists on router %s", name, router)), nil
	}
	nats = append(nats, natBodyFromProps(props))

	resp, err := p.patchNats(ctx, client, cfg.Project, region, router, nats)
	if err != nil {
		wrapped := transport.WrapError(err, "failed to patch router")
		return createFailure(transport.ToResourceErrorCode(wrapped.Code), wrapped.Message), nil
	}

	nativeID := buildNatNativeID(cfg.Project, region, router, name)
	pathCtx := base.PathContext{Project: cfg.Project, Region: region, ResourceType: "routers"}
	opID := p.OperationConfig.OperationIDExtractor(resp.Body)
	opPath := p.OperationConfig.OperationURLBuilder(pathCtx, opID)

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        nativeID,
			RequestID:       encodeNatRequestID(opPath, nativeID),
			StatusMessage:   "RouterNat creation in progress",
		},
	}, nil
}

func (p *RouterNatProvisioner) Read(
	ctx context.Context, request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	project, region, router, natName, err := parseNatNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}

	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	parent, err := p.fetchRouter(ctx, client, project, region, router)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeServiceInternalError}, nil
	}
	if parent == nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
	}

	nats := natsOf(parent)
	idx := findNat(nats, natName)
	if idx < 0 {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
	}

	nat, _ := nats[idx].(map[string]interface{})
	// Re-attach formae-level fields stripped from the wire payload.
	out := make(map[string]interface{}, len(nat)+2)
	for k, v := range nat {
		out[k] = v
	}
	out["router"] = router
	out["region"] = region

	propsJSON, _ := json.Marshal(out)
	return &resource.ReadResult{Properties: string(propsJSON)}, nil
}

func (p *RouterNatProvisioner) Update(
	ctx context.Context, request *resource.UpdateRequest,
) (*resource.UpdateResult, error) {
	project, region, router, natName, err := parseNatNativeID(request.NativeID)
	if err != nil {
		return updateFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	var desired map[string]interface{}
	if err := json.Unmarshal(request.DesiredProperties, &desired); err != nil {
		return updateFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid desired properties: %v", err)), nil
	}

	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	parent, err := p.fetchRouter(ctx, client, project, region, router)
	if err != nil {
		return updateFailure(resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}
	if parent == nil {
		return updateFailure(resource.OperationErrorCodeNotFound,
			fmt.Sprintf("router %s not found", router)), nil
	}

	nats := natsOf(parent)
	idx := findNat(nats, natName)
	if idx < 0 {
		return updateFailure(resource.OperationErrorCodeNotFound,
			fmt.Sprintf("nat %s not found on router %s", natName, router)), nil
	}

	// formae sends the full desired body, so replace the slot wholesale.
	// Sibling NATs at other indices are untouched.
	nats[idx] = natBodyFromProps(desired)

	resp, err := p.patchNats(ctx, client, project, region, router, nats)
	if err != nil {
		wrapped := transport.WrapError(err, "failed to patch router")
		return updateFailure(transport.ToResourceErrorCode(wrapped.Code), wrapped.Message), nil
	}

	nativeID := buildNatNativeID(project, region, router, natName)
	pathCtx := base.PathContext{Project: project, Region: region, ResourceType: "routers"}
	opID := p.OperationConfig.OperationIDExtractor(resp.Body)
	opPath := p.OperationConfig.OperationURLBuilder(pathCtx, opID)

	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        nativeID,
			RequestID:       encodeNatRequestID(opPath, nativeID),
			StatusMessage:   "RouterNat update in progress",
		},
	}, nil
}

// Delete is idempotent — if the NAT or parent router is already gone, success.
func (p *RouterNatProvisioner) Delete(
	ctx context.Context, request *resource.DeleteRequest,
) (*resource.DeleteResult, error) {
	project, region, router, natName, err := parseNatNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}

	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	parent, err := p.fetchRouter(ctx, client, project, region, router)
	if err != nil {
		return deleteFailure(resource.OperationErrorCodeServiceInternalError, err.Error()), nil
	}
	if parent == nil {
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusSuccess,
				StatusMessage:   "parent router already deleted",
			},
		}, nil
	}

	nats := natsOf(parent)
	idx := findNat(nats, natName)
	if idx < 0 {
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusSuccess,
				StatusMessage:   "RouterNat already absent",
			},
		}, nil
	}

	pruned := append(append([]interface{}{}, nats[:idx]...), nats[idx+1:]...)

	resp, err := p.patchNats(ctx, client, project, region, router, pruned)
	if err != nil {
		wrapped := transport.WrapError(err, "failed to patch router")
		return deleteFailure(transport.ToResourceErrorCode(wrapped.Code), wrapped.Message), nil
	}

	nativeID := buildNatNativeID(project, region, router, natName)
	pathCtx := base.PathContext{Project: project, Region: region, ResourceType: "routers"}
	opID := p.OperationConfig.OperationIDExtractor(resp.Body)
	opPath := p.OperationConfig.OperationURLBuilder(pathCtx, opID)

	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        nativeID,
			RequestID:       encodeNatRequestID(opPath, nativeID),
			StatusMessage:   "RouterNat deletion in progress",
		},
	}, nil
}

// List enumerates every NAT across every Router in the configured region.
func (p *RouterNatProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	cfg := config.FromTargetConfig(request.TargetConfig)
	if cfg.Region == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	url := fmt.Sprintf("%s/projects/%s/regions/%s/routers",
		p.APIConfig.BaseURL, cfg.Project, cfg.Region)

	resp, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: url})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to list routers")
		return nil, errors.New(wrapped.Message)
	}

	items, _ := resp.Body["items"].([]interface{})
	out := make([]string, 0, len(items))
	for _, raw := range items {
		router, _ := raw.(map[string]interface{})
		routerName := utils.GetString(router, "name")
		for _, n := range natsOf(router) {
			nat, _ := n.(map[string]interface{})
			natName := utils.GetString(nat, "name")
			if routerName != "" && natName != "" {
				out = append(out, buildNatNativeID(cfg.Project, cfg.Region, routerName, natName))
			}
		}
	}
	return &resource.ListResult{NativeIDs: out}, nil
}

// Status overrides BaseResource.Status because routers.patch reports a
// targetLink pointing at the parent Router — base.Status would surface that as
// the NativeID and mask the NAT's identity. We pull the synthetic NAT ID back
// out of RequestID instead.
func (p *RouterNatProvisioner) Status(
	ctx context.Context, request *resource.StatusRequest,
) (*resource.StatusResult, error) {
	opPath, natID := parseNatRequestID(request.RequestID)
	if opPath == "" {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
				StatusMessage:   "RequestID missing operation path",
				RequestID:       request.RequestID,
			},
		}, nil
	}

	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	url := fmt.Sprintf("%s/%s", p.APIConfig.BaseURL, opPath)
	resp, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: url})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to get operation status")
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       transport.ToResourceErrorCode(wrapped.Code),
				StatusMessage:   wrapped.Message,
				RequestID:       request.RequestID,
				NativeID:        natID,
			},
		}, nil
	}

	done, checkErr := p.OperationConfig.OperationStatusChecker(resp.Body)
	if checkErr != nil {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeServiceInternalError,
				StatusMessage:   checkErr.Error(),
				RequestID:       request.RequestID,
				NativeID:        natID,
			},
		}, nil
	}
	if !done {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusInProgress,
				StatusMessage:   "Operation in progress",
				RequestID:       request.RequestID,
				NativeID:        natID,
			},
		}, nil
	}
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
			StatusMessage:   "Operation completed successfully",
			RequestID:       request.RequestID,
			NativeID:        natID,
		},
	}, nil
}

func createFailure(code resource.OperationErrorCode, msg string) *resource.CreateResult {
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       code,
			StatusMessage:   msg,
		},
	}
}

func updateFailure(code resource.OperationErrorCode, msg string) *resource.UpdateResult {
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       code,
			StatusMessage:   msg,
		},
	}
}

func deleteFailure(code resource.OperationErrorCode, msg string) *resource.DeleteResult {
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       code,
			StatusMessage:   msg,
		},
	}
}
