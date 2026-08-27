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

// RouterSubResourceProvisioner manages one BGP object owned by a Cloud Router:
// a route policy (which filters or rewrites the routes the router imports and
// advertises) or a named set (a reusable list of prefixes those policies match
// against). Neither is a REST resource; both are sets of verbs on the router.
//
// The two are the same shape down to their quirks — the update verb also
// creates, the get verb wraps its payload in a "resource" envelope, the list
// verb returns "result" instead of "items", and an update must carry the
// current fingerprint while a create must not carry one at all — so they are one
// provisioner parameterised by routerSubKind.
type RouterSubResourceProvisioner struct {
	*base.BaseResource
	kind routerSubKind
}

var _ prov.Provisioner = (*RouterSubResourceProvisioner)(nil)

// routerSubKind is everything that differs between the two.
type routerSubKind struct {
	// segment is the sub-collection in the native ID.
	segment string
	// The four verbs, which do not follow one naming rule: the list verb
	// pluralises the noun ("listRoutePolicies", "listNamedSets").
	updateVerb string
	getVerb    string
	deleteVerb string
	listVerb   string
	// queryParam names the object in get/delete URLs.
	queryParam string
	label      string
}

var (
	routePolicyKind = routerSubKind{
		segment:    "routePolicies",
		updateVerb: "updateRoutePolicy",
		getVerb:    "getRoutePolicy",
		deleteVerb: "deleteRoutePolicy",
		listVerb:   "listRoutePolicies",
		queryParam: "policy",
		label:      "router route policy",
	}
	namedSetKind = routerSubKind{
		segment:    "namedSets",
		updateVerb: "updateNamedSet",
		getVerb:    "getNamedSet",
		deleteVerb: "deleteNamedSet",
		listVerb:   "listNamedSets",
		queryParam: "namedSet",
		label:      "router named set",
	}
)

// routerSubRouterProperty names the owning router; it and "region" are path
// components, never part of the verb body.
const routerSubRouterProperty = "router"

func init() {
	registerRouterSubResource(RouterRoutePolicyResourceType, routePolicyKind)
	registerRouterSubResource(RouterNamedSetResourceType, namedSetKind)
}

func registerRouterSubResource(resourceType string, kind routerSubKind) {
	registry.Register(resourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(cfg *config.Config) prov.Provisioner {
			return &RouterSubResourceProvisioner{
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
				kind: kind,
			}
		})
}

// nativeID composes
// "projects/{p}/regions/{r}/routers/{router}/{segment}/{name}".
func (k routerSubKind) nativeID(project, region, router, name string) string {
	return fmt.Sprintf("projects/%s/regions/%s/routers/%s/%s/%s",
		project, region, router, k.segment, name)
}

// parseNativeID splits the composite id. The object is addressed by
// (router, name) within a region, so all of it has to survive, and the segment
// must match this kind — a named set read through the route-policy verbs would
// simply not be found.
func (k routerSubKind) parseNativeID(nativeID string) (project, region, router, name string, err error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 8 || parts[0] != "projects" || parts[2] != "regions" ||
		parts[3] == "" || parts[4] != "routers" || parts[6] != k.segment || parts[7] == "" {
		return "", "", "", "", fmt.Errorf("invalid %s native ID: %s", k.label, nativeID)
	}
	return parts[1], parts[3], parts[5], parts[7], nil
}

// routerSubBody keeps only what the verb accepts. The router and region address
// the URL, and the fingerprint is supplied by Update from a fresh read — never
// from the declared forma, which would go stale.
func routerSubBody(props map[string]interface{}) map[string]interface{} {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		switch k {
		case routerSubRouterProperty, "region", "fingerprint":
			continue
		}
		body[k] = v
	}
	return body
}

func (p *RouterSubResourceProvisioner) routerURL(project, region, router string) string {
	return fmt.Sprintf("%s/projects/%s/regions/%s/routers/%s",
		p.APIConfig.BaseURL, project, region, router)
}

func (p *RouterSubResourceProvisioner) projectFor(targetConfig json.RawMessage, fallback string) string {
	if cfg := config.FromTargetConfig(targetConfig, nil /* path context only; this config never authenticates */); cfg != nil && cfg.Project != "" {
		return cfg.Project
	}
	return fallback
}

func (p *RouterSubResourceProvisioner) regionFor(props map[string]interface{}, targetConfig json.RawMessage, fallback string) string {
	if region, ok := props["region"].(string); ok && region != "" {
		return region
	}
	if cfg := config.FromTargetConfig(targetConfig, nil /* path context only; this config never authenticates */); cfg != nil && cfg.Region != "" {
		return cfg.Region
	}
	return fallback
}

func (p *RouterSubResourceProvisioner) issueVerb(
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
		return "", transport.WrapError(err, p.kind.label+" verb failed")
	}
	opID := p.OperationConfig.OperationIDExtractor(resp.Body)
	return p.OperationConfig.OperationURLBuilder(
		base.PathContext{Project: project, Region: region}, opID), nil
}

// fetch reads one object, unwrapping the "resource" envelope the API puts it in.
// A missing one may answer 400 ("The policy does not exist") or 404
// ("NAMED_SET_NOT_FOUND"), so both spellings are treated as gone.
func (p *RouterSubResourceProvisioner) fetch(
	ctx context.Context, project, region, router, name string,
) (policy map[string]interface{}, gone bool, err *transport.Error) {
	client, cErr := transport.NewClient(ctx, p.Config)
	if cErr != nil {
		return nil, false, transport.WrapError(cErr, "failed to create transport client")
	}
	resp, rErr := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL: fmt.Sprintf("%s/%s?%s=%s", p.routerURL(project, region, router),
			p.kind.getVerb, p.kind.queryParam, name),
	})
	if rErr != nil {
		wrapped := transport.WrapError(rErr, "failed to read "+p.kind.label)
		code := transport.ToResourceErrorCode(wrapped.Code)
		msg := strings.ToLower(wrapped.Message)
		if code == resource.OperationErrorCodeNotFound ||
			strings.Contains(msg, "does not exist") || strings.Contains(msg, "not found") {
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

func (p *RouterSubResourceProvisioner) Create(
	ctx context.Context, request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid properties: %v", err)), nil
	}
	router, _ := props[routerSubRouterProperty].(string)
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
		p.routerURL(project, region, router)+"/"+p.kind.updateVerb,
		routerSubBody(props), project, region)
	if verbErr != nil {
		return createFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        p.kind.nativeID(project, region, router, name),
			RequestID:       requestID,
			StatusMessage:   p.kind.label + " creation in progress",
		},
	}, nil
}

func (p *RouterSubResourceProvisioner) Read(
	ctx context.Context, request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	project, region, router, name, err := p.kind.parseNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}
	project = p.projectFor(request.TargetConfig, project)

	policy, gone, vErr := p.fetch(ctx, project, region, router, name)
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
	props[routerSubRouterProperty] = router
	props["region"] = region

	encoded, mErr := json.Marshal(props)
	if mErr != nil {
		return nil, fmt.Errorf("failed to marshal route policy properties: %w", mErr)
	}
	return &resource.ReadResult{Properties: string(encoded)}, nil
}

// Update re-reads the policy for its current fingerprint: the verb requires one
// and rejects a stale value, so it cannot come from the declared forma.
func (p *RouterSubResourceProvisioner) Update(
	ctx context.Context, request *resource.UpdateRequest,
) (*resource.UpdateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.DesiredProperties, &props); err != nil {
		return updateFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid properties: %v", err)), nil
	}
	project, region, router, name, err := p.kind.parseNativeID(request.NativeID)
	if err != nil {
		return updateFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	project = p.projectFor(request.TargetConfig, project)

	current, gone, vErr := p.fetch(ctx, project, region, router, name)
	if vErr != nil {
		return updateFailure(transport.ToResourceErrorCode(vErr.Code), vErr.Message), nil
	}
	if gone {
		return updateFailure(resource.OperationErrorCodeNotFound,
			p.kind.label+" no longer exists"), nil
	}

	body := routerSubBody(props)
	if fingerprint, ok := current["fingerprint"].(string); ok && fingerprint != "" {
		body["fingerprint"] = fingerprint
	}

	requestID, verbErr := p.issueVerb(ctx,
		p.routerURL(project, region, router)+"/"+p.kind.updateVerb, body, project, region)
	if verbErr != nil {
		return updateFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        request.NativeID,
			RequestID:       requestID,
			StatusMessage:   p.kind.label + " update in progress",
		},
	}, nil
}

func (p *RouterSubResourceProvisioner) Delete(
	ctx context.Context, request *resource.DeleteRequest,
) (*resource.DeleteResult, error) {
	project, region, router, name, err := p.kind.parseNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	project = p.projectFor(request.TargetConfig, project)

	url := fmt.Sprintf("%s/%s?%s=%s", p.routerURL(project, region, router),
		p.kind.deleteVerb, p.kind.queryParam, name)
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
			StatusMessage:   p.kind.label + " deletion in progress",
		},
	}, nil
}

// List enumerates one router's policies, which arrive under "result" rather than
// the usual "items". Policies live inside their router, so discovery has to be
// told which router to look in.
func (p *RouterSubResourceProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	router := ""
	region := ""
	if request.AdditionalProperties != nil {
		router = request.AdditionalProperties[routerSubRouterProperty]
		region = request.AdditionalProperties["region"]
	}
	project := p.projectFor(request.TargetConfig, "")
	region = p.regionFor(map[string]interface{}{"region": region}, request.TargetConfig, "")
	if project == "" || region == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	// A named router is the caller telling us where to look. Discovery names
	// none, and these objects live inside their router rather than in a
	// collection of their own, so the routers have to be walked first.
	routers := []string{router}
	if router == "" {
		routers, err = listComputeCollectionNames(ctx, client,
			fmt.Sprintf("%s/projects/%s/regions/%s/routers", p.APIConfig.BaseURL, project, region),
			"routers")
		if err != nil {
			return nil, err
		}
	}

	nativeIDs := []string{}
	for _, name := range routers {
		resp, rErr := client.SendRequest(ctx, transport.RequestOptions{
			Method: "GET",
			URL:    p.routerURL(project, region, name) + "/" + p.kind.listVerb,
		})
		if rErr != nil {
			if router != "" {
				wrapped := transport.WrapError(rErr, "failed to list "+p.kind.label+"s")
				return nil, fmt.Errorf("%s", wrapped.Message)
			}
			// One unreadable router must not hide the rest.
			continue
		}
		objects, _ := resp.Body["result"].([]interface{})
		for _, entry := range objects {
			object, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}
			if objName, ok := object["name"].(string); ok && objName != "" {
				nativeIDs = append(nativeIDs, p.kind.nativeID(project, region, name, objName))
			}
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// Status routes through the shared read-back so post-create and post-update
// state carries the resource's real properties, not just what was declared.
func (p *RouterSubResourceProvisioner) Status(
	ctx context.Context, request *resource.StatusRequest,
) (*resource.StatusResult, error) {
	return base.StatusWithRead(ctx, p.BaseResource, p.Read, request)
}
