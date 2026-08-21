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

// RouterRoutePolicyProvisioner manages one BGP route policy on a Cloud Router.
// A policy filters or transforms the routes a router imports or advertises, so
// it is how a router does anything selective with BGP.
//
// The policy is not a REST resource but a set of verbs on the router:
// updateRoutePolicy (which both creates and updates), getRoutePolicy?policy=N,
// deleteRoutePolicy?policy=N and listRoutePolicies. Two quirks shape this code:
// getRoutePolicy wraps the policy in a "resource" envelope, and an update must
// carry the current fingerprint while a create must not have one at all.
type RouterRoutePolicyProvisioner struct {
	*base.BaseResource
}

var _ prov.Provisioner = (*RouterRoutePolicyProvisioner)(nil)

// routePolicyRouterProperty names the owning router; it and "region" are path
// components, never part of the verb body.
const routePolicyRouterProperty = "router"

func init() {
	registry.Register(RouterRoutePolicyResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(cfg *config.Config) prov.Provisioner {
			return &RouterRoutePolicyProvisioner{
				BaseResource: &base.BaseResource{
					Config:          cfg,
					APIConfig:       ComputeAPI,
					OperationConfig: ComputeOperations,
					ResourceConfig: base.ResourceConfig{
						ResourceType: "routers",
						Scope:        &base.ScopeConfig{Type: base.ScopeRegional},
					},
					NativeIDConfig: ComputeNativeID,
				},
			}
		})
}

// buildRoutePolicyNativeID composes
// "projects/{p}/regions/{r}/routers/{router}/routePolicies/{name}".
func buildRoutePolicyNativeID(project, region, router, name string) string {
	return fmt.Sprintf("projects/%s/regions/%s/routers/%s/routePolicies/%s",
		project, region, router, name)
}

// parseRoutePolicyNativeID splits the composite id. A policy is addressed by
// (router, name) within a region, so all of it has to survive.
func parseRoutePolicyNativeID(nativeID string) (project, region, router, name string, err error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 8 || parts[0] != "projects" || parts[2] != "regions" ||
		parts[3] == "" || parts[4] != "routers" || parts[6] != "routePolicies" || parts[7] == "" {
		return "", "", "", "", fmt.Errorf("invalid router route policy native ID: %s", nativeID)
	}
	return parts[1], parts[3], parts[5], parts[7], nil
}

// routePolicyBody keeps only what the verb accepts. The router and region
// address the URL, and the fingerprint is supplied by Update from a fresh read —
// never from the declared forma, which would go stale.
func routePolicyBody(props map[string]interface{}) map[string]interface{} {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		switch k {
		case routePolicyRouterProperty, "region", "fingerprint":
			continue
		}
		body[k] = v
	}
	return body
}

func (p *RouterRoutePolicyProvisioner) routerURL(project, region, router string) string {
	return fmt.Sprintf("%s/projects/%s/regions/%s/routers/%s",
		p.APIConfig.BaseURL, project, region, router)
}

func (p *RouterRoutePolicyProvisioner) projectFor(targetConfig json.RawMessage, fallback string) string {
	if cfg := config.FromTargetConfig(targetConfig); cfg != nil && cfg.Project != "" {
		return cfg.Project
	}
	return fallback
}

func (p *RouterRoutePolicyProvisioner) regionFor(props map[string]interface{}, targetConfig json.RawMessage, fallback string) string {
	if region, ok := props["region"].(string); ok && region != "" {
		return region
	}
	if cfg := config.FromTargetConfig(targetConfig); cfg != nil && cfg.Region != "" {
		return cfg.Region
	}
	return fallback
}

func (p *RouterRoutePolicyProvisioner) issueVerb(
	ctx context.Context, url string, body map[string]interface{}, project, region string,
) (string, *transport.Error) {
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return "", transport.WrapError(err, "failed to create transport client")
	}
	resp, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "POST",
		URL:    url,
		Body:   body,
	})
	if err != nil {
		return "", transport.WrapError(err, "router route policy verb failed")
	}
	opID := p.OperationConfig.OperationIDExtractor(resp.Body)
	return p.OperationConfig.OperationURLBuilder(
		base.PathContext{Project: project, Region: region}, opID), nil
}

// fetchRoutePolicy reads one policy, unwrapping the "resource" envelope the API
// puts it in. A missing policy answers 400 ("The policy does not exist"), not
// 404, so the caller is told plainly whether it is gone.
func (p *RouterRoutePolicyProvisioner) fetchRoutePolicy(
	ctx context.Context, project, region, router, name string,
) (policy map[string]interface{}, gone bool, err *transport.Error) {
	client, cErr := transport.NewClient(ctx, p.Config)
	if cErr != nil {
		return nil, false, transport.WrapError(cErr, "failed to create transport client")
	}
	resp, rErr := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL: fmt.Sprintf("%s/getRoutePolicy?policy=%s",
			p.routerURL(project, region, router), name),
	})
	if rErr != nil {
		wrapped := transport.WrapError(rErr, "failed to read router route policy")
		code := transport.ToResourceErrorCode(wrapped.Code)
		if code == resource.OperationErrorCodeNotFound ||
			(code == resource.OperationErrorCodeInvalidRequest &&
				strings.Contains(strings.ToLower(wrapped.Message), "does not exist")) {
			return nil, true, nil
		}
		return nil, false, wrapped
	}
	inner, ok := resp.Body["resource"].(map[string]interface{})
	if !ok {
		// No envelope means no policy to report.
		return nil, true, nil
	}
	return inner, false, nil
}

func (p *RouterRoutePolicyProvisioner) Create(
	ctx context.Context, request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid properties: %v", err)), nil
	}
	router, _ := props[routePolicyRouterProperty].(string)
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

	// A create must not carry a fingerprint; the API rejects one for a policy
	// that does not exist yet.
	requestID, verbErr := p.issueVerb(ctx,
		p.routerURL(project, region, router)+"/updateRoutePolicy",
		routePolicyBody(props), project, region)
	if verbErr != nil {
		return createFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        buildRoutePolicyNativeID(project, region, router, name),
			RequestID:       requestID,
			StatusMessage:   "router route policy creation in progress",
		},
	}, nil
}

func (p *RouterRoutePolicyProvisioner) Read(
	ctx context.Context, request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	project, region, router, name, err := parseRoutePolicyNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}
	project = p.projectFor(request.TargetConfig, project)

	policy, gone, vErr := p.fetchRoutePolicy(ctx, project, region, router, name)
	if vErr != nil {
		return &resource.ReadResult{ErrorCode: transport.ToResourceErrorCode(vErr.Code)}, nil
	}
	if gone {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
	}

	// The router and region are path components, so put them back for
	// comparison against the declared forma.
	props := make(map[string]interface{}, len(policy)+2)
	for k, v := range policy {
		props[k] = v
	}
	props[routePolicyRouterProperty] = router
	props["region"] = region

	encoded, mErr := json.Marshal(props)
	if mErr != nil {
		return nil, fmt.Errorf("failed to marshal route policy properties: %w", mErr)
	}
	return &resource.ReadResult{Properties: string(encoded)}, nil
}

// Update re-reads the policy for its current fingerprint: the verb requires one
// and rejects a stale value, so it cannot come from the declared forma.
func (p *RouterRoutePolicyProvisioner) Update(
	ctx context.Context, request *resource.UpdateRequest,
) (*resource.UpdateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.DesiredProperties, &props); err != nil {
		return updateFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid properties: %v", err)), nil
	}
	project, region, router, name, err := parseRoutePolicyNativeID(request.NativeID)
	if err != nil {
		return updateFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	project = p.projectFor(request.TargetConfig, project)

	current, gone, vErr := p.fetchRoutePolicy(ctx, project, region, router, name)
	if vErr != nil {
		return updateFailure(transport.ToResourceErrorCode(vErr.Code), vErr.Message), nil
	}
	if gone {
		return updateFailure(resource.OperationErrorCodeNotFound,
			"route policy no longer exists"), nil
	}

	body := routePolicyBody(props)
	if fingerprint, ok := current["fingerprint"].(string); ok && fingerprint != "" {
		body["fingerprint"] = fingerprint
	}

	requestID, verbErr := p.issueVerb(ctx,
		p.routerURL(project, region, router)+"/updateRoutePolicy", body, project, region)
	if verbErr != nil {
		return updateFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        request.NativeID,
			RequestID:       requestID,
			StatusMessage:   "router route policy update in progress",
		},
	}, nil
}

func (p *RouterRoutePolicyProvisioner) Delete(
	ctx context.Context, request *resource.DeleteRequest,
) (*resource.DeleteResult, error) {
	project, region, router, name, err := parseRoutePolicyNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	project = p.projectFor(request.TargetConfig, project)

	url := fmt.Sprintf("%s/deleteRoutePolicy?policy=%s",
		p.routerURL(project, region, router), name)
	requestID, verbErr := p.issueVerb(ctx, url, nil, project, region)
	if verbErr != nil {
		return deleteFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        request.NativeID,
			RequestID:       requestID,
			StatusMessage:   "router route policy deletion in progress",
		},
	}, nil
}

// List enumerates one router's policies, which arrive under "result" rather than
// the usual "items". Policies live inside their router, so discovery has to be
// told which router to look in.
func (p *RouterRoutePolicyProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	router := ""
	region := ""
	if request.AdditionalProperties != nil {
		router = request.AdditionalProperties[routePolicyRouterProperty]
		region = request.AdditionalProperties["region"]
	}
	project := p.projectFor(request.TargetConfig, "")
	region = p.regionFor(map[string]interface{}{"region": region}, request.TargetConfig, "")
	if router == "" || project == "" || region == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}
	resp, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    p.routerURL(project, region, router) + "/listRoutePolicies",
	})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to list router route policies")
		return nil, fmt.Errorf("%s", wrapped.Message)
	}

	policies, _ := resp.Body["result"].([]interface{})
	nativeIDs := make([]string, 0, len(policies))
	for _, entry := range policies {
		policy, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		if name, ok := policy["name"].(string); ok && name != "" {
			nativeIDs = append(nativeIDs, buildRoutePolicyNativeID(project, region, router, name))
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// Status routes through the shared read-back so post-create and post-update
// state carries the resource's real properties, not just what was declared.
func (p *RouterRoutePolicyProvisioner) Status(
	ctx context.Context, request *resource.StatusRequest,
) (*resource.StatusResult, error) {
	return base.StatusWithRead(ctx, p.BaseResource, p.Read, request)
}
