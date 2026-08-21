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

// RouterInterfaceProvisioner manages one entry of Router.interfaces[]. An
// interface is where a Cloud Router attaches to something it can peer over — a
// VPN tunnel, an interconnect attachment, or a subnet for a router appliance —
// and a BGP peer is configured against one, so this is the first half of making
// a router speak BGP.
//
// Like Cloud NAT it is not a standalone REST resource: it lives in the router
// and is managed by read-modify-write through routers.patch. Sibling interfaces
// on the same router survive every operation.
type RouterInterfaceProvisioner struct {
	*base.BaseResource
}

var _ prov.Provisioner = (*RouterInterfaceProvisioner)(nil)

func init() {
	registry.Register(RouterInterfaceResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(cfg *config.Config) prov.Provisioner {
			return &RouterInterfaceProvisioner{
				BaseResource: &base.BaseResource{
					Config:          cfg,
					OperationConfig: ComputeOperations,
					APIConfig:       ComputeAPI,
					// Parent type, for URL building only - an interface has no
					// REST type of its own.
					ResourceConfig: base.ResourceConfig{
						ResourceType: "routers",
						Scope:        &base.ScopeConfig{Type: base.ScopeRegional},
					},
					NativeIDConfig: ComputeNativeID,
				},
			}
		})
}

// routerInterfacePathProps are formae-side or path-side keys that must not go
// into the interface body.
var routerInterfacePathProps = map[string]bool{"router": true, "region": true}

func buildRouterInterfaceNativeID(project, region, router, name string) string {
	return fmt.Sprintf("projects/%s/regions/%s/routers/%s/interfaces/%s",
		project, region, router, name)
}

func parseRouterInterfaceNativeID(nativeID string) (project, region, router, name string, err error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 8 || parts[0] != "projects" || parts[2] != "regions" || parts[3] == "" ||
		parts[4] != "routers" || parts[6] != "interfaces" || parts[7] == "" {
		return "", "", "", "", fmt.Errorf("invalid router interface native ID: %s", nativeID)
	}
	return parts[1], parts[3], parts[5], parts[7], nil
}

// routerInterfaceBody strips the keys that address the interface rather than
// describe it.
func routerInterfaceBody(props map[string]interface{}) map[string]interface{} {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		if routerInterfacePathProps[k] {
			continue
		}
		body[k] = v
	}
	return body
}

// mergeRouterInterface sets one named interface in the list, leaving the others
// untouched and in order. routers.patch replaces the whole array, so anything
// dropped here is dropped from the router.
func mergeRouterInterface(interfaces []interface{}, name string, iface map[string]interface{}, remove bool) []interface{} {
	merged := make([]interface{}, 0, len(interfaces)+1)
	found := false
	for _, raw := range interfaces {
		existing, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if existingName, _ := existing["name"].(string); existingName == name {
			found = true
			if remove {
				continue
			}
			merged = append(merged, iface)
			continue
		}
		merged = append(merged, existing)
	}
	if !found && !remove {
		merged = append(merged, iface)
	}
	return merged
}

// findRouterInterface returns one interface from a router body.
func findRouterInterface(router map[string]interface{}, name string) map[string]interface{} {
	interfaces, _ := router["interfaces"].([]interface{})
	for _, raw := range interfaces {
		iface, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if ifaceName, _ := iface["name"].(string); ifaceName == name {
			return iface
		}
	}
	return nil
}

func (p *RouterInterfaceProvisioner) routerURL(project, region, router string) string {
	return fmt.Sprintf("%s/projects/%s/regions/%s/routers/%s",
		p.APIConfig.BaseURL, project, region, router)
}

func (p *RouterInterfaceProvisioner) projectFor(targetConfig json.RawMessage, fallback string) string {
	if cfg := config.FromTargetConfig(targetConfig); cfg != nil && cfg.Project != "" {
		return cfg.Project
	}
	return fallback
}

func (p *RouterInterfaceProvisioner) regionFor(props map[string]interface{}, targetConfig json.RawMessage, fallback string) string {
	if region, ok := props["region"].(string); ok && region != "" {
		return region
	}
	if cfg := config.FromTargetConfig(targetConfig); cfg != nil && cfg.Region != "" {
		return cfg.Region
	}
	return fallback
}

func (p *RouterInterfaceProvisioner) fetchRouter(
	ctx context.Context, project, region, router string,
) (map[string]interface{}, bool, *transport.Error) {
	client, cErr := transport.NewClient(ctx, p.Config)
	if cErr != nil {
		return nil, false, transport.WrapError(cErr, "failed to create transport client")
	}
	resp, rErr := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    p.routerURL(project, region, router),
	})
	if rErr != nil {
		wrapped := transport.WrapError(rErr, "failed to read router")
		if transport.ToResourceErrorCode(wrapped.Code) == resource.OperationErrorCodeNotFound {
			return nil, true, nil
		}
		return nil, false, wrapped
	}
	return resp.Body, false, nil
}

// patchInterfaces writes the merged list back. The patch names only
// "interfaces", so the router's NATs and BGP peers are left alone.
func (p *RouterInterfaceProvisioner) patchInterfaces(
	ctx context.Context, project, region, router string, interfaces []interface{},
) (string, *transport.Error) {
	client, cErr := transport.NewClient(ctx, p.Config)
	if cErr != nil {
		return "", transport.WrapError(cErr, "failed to create transport client")
	}
	resp, sErr := client.SendRequest(ctx, transport.RequestOptions{
		Method: "PATCH",
		URL:    p.routerURL(project, region, router),
		Body:   map[string]interface{}{"interfaces": interfaces},
	})
	if sErr != nil {
		return "", transport.WrapError(sErr, "failed to patch router interfaces")
	}
	opID := p.OperationConfig.OperationIDExtractor(resp.Body)
	return p.OperationConfig.OperationURLBuilder(
		base.PathContext{Project: project, Region: region}, opID), nil
}

// writeInterface is the shared read-merge-patch used by create, update and
// delete.
func (p *RouterInterfaceProvisioner) writeInterface(
	ctx context.Context, project, region, router, name string,
	iface map[string]interface{}, remove bool,
) (string, *transport.Error) {
	current, gone, fErr := p.fetchRouter(ctx, project, region, router)
	if fErr != nil {
		return "", fErr
	}
	if gone {
		return "", &transport.Error{
			Code:    transport.ErrorCodeResourceNotFound,
			Message: fmt.Sprintf("router %s not found", router),
		}
	}
	existing, _ := current["interfaces"].([]interface{})
	return p.patchInterfaces(ctx, project, region, router,
		mergeRouterInterface(existing, name, iface, remove))
}

func (p *RouterInterfaceProvisioner) Create(
	ctx context.Context, request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid properties: %v", err)), nil
	}
	router, _ := props["router"].(string)
	name, _ := props["name"].(string)
	if router == "" || name == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"router and name are required"), nil
	}
	project := p.projectFor(request.TargetConfig, "")
	region := p.regionFor(props, request.TargetConfig, "")
	if project == "" || region == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"target project and region are required"), nil
	}

	requestID, wErr := p.writeInterface(ctx, project, region, router, name,
		routerInterfaceBody(props), false)
	if wErr != nil {
		return createFailure(transport.ToResourceErrorCode(wErr.Code), wErr.Message), nil
	}
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        buildRouterInterfaceNativeID(project, region, router, name),
			RequestID:       requestID,
			StatusMessage:   "router interface creation in progress",
		},
	}, nil
}

func (p *RouterInterfaceProvisioner) Read(
	ctx context.Context, request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	project, region, router, name, err := parseRouterInterfaceNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}
	project = p.projectFor(request.TargetConfig, project)

	current, gone, fErr := p.fetchRouter(ctx, project, region, router)
	if fErr != nil {
		return &resource.ReadResult{ErrorCode: transport.ToResourceErrorCode(fErr.Code)}, nil
	}
	if gone {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
	}
	iface := findRouterInterface(current, name)
	if iface == nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
	}

	// The router and region address the URL, so put them back for comparison
	// against the declared forma.
	props := make(map[string]interface{}, len(iface)+2)
	for k, v := range iface {
		props[k] = v
	}
	props["router"] = router
	props["region"] = region

	encoded, mErr := json.Marshal(props)
	if mErr != nil {
		return nil, fmt.Errorf("failed to marshal router interface properties: %w", mErr)
	}
	return &resource.ReadResult{Properties: string(encoded)}, nil
}

// Update is not possible: the API rejects an in-place change to an existing
// interface, so formae replaces it instead - Delete then Create, which the
// merge handles without touching sibling interfaces.
func (p *RouterInterfaceProvisioner) Update(
	ctx context.Context, request *resource.UpdateRequest,
) (*resource.UpdateResult, error) {
	return updateFailure(resource.OperationErrorCodeNotUpdatable,
		"a router interface cannot be changed in place; a change replaces it"), nil
}

func (p *RouterInterfaceProvisioner) Delete(
	ctx context.Context, request *resource.DeleteRequest,
) (*resource.DeleteResult, error) {
	project, region, router, name, err := parseRouterInterfaceNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	project = p.projectFor(request.TargetConfig, project)

	requestID, wErr := p.writeInterface(ctx, project, region, router, name, nil, true)
	if wErr != nil {
		// A router that is already gone took its interfaces with it.
		if transport.ToResourceErrorCode(wErr.Code) == resource.OperationErrorCodeNotFound {
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusSuccess,
					NativeID:        request.NativeID,
					StatusMessage:   "router already deleted",
				},
			}, nil
		}
		return deleteFailure(transport.ToResourceErrorCode(wErr.Code), wErr.Message), nil
	}
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        request.NativeID,
			RequestID:       requestID,
			StatusMessage:   "router interface deletion in progress",
		},
	}, nil
}

// List enumerates one router's interfaces, so discovery has to be told which
// router to look in.
func (p *RouterInterfaceProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	router := ""
	region := ""
	if request.AdditionalProperties != nil {
		router = request.AdditionalProperties["router"]
		region = request.AdditionalProperties["region"]
	}
	project := p.projectFor(request.TargetConfig, "")
	region = p.regionFor(map[string]interface{}{"region": region}, request.TargetConfig, "")
	if router == "" || project == "" || region == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	current, gone, fErr := p.fetchRouter(ctx, project, region, router)
	if fErr != nil {
		return nil, fmt.Errorf("%s", fErr.Message)
	}
	if gone {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}
	interfaces, _ := current["interfaces"].([]interface{})
	nativeIDs := make([]string, 0, len(interfaces))
	for _, raw := range interfaces {
		iface, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if name, _ := iface["name"].(string); name != "" {
			nativeIDs = append(nativeIDs, buildRouterInterfaceNativeID(project, region, router, name))
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// Status routes through the shared read-back so post-create and post-update
// state carries the interface the API actually built - it fills ipRange and
// ipVersion itself.
func (p *RouterInterfaceProvisioner) Status(
	ctx context.Context, request *resource.StatusRequest,
) (*resource.StatusResult, error) {
	return base.StatusWithRead(ctx, p.BaseResource, p.Read, request)
}
