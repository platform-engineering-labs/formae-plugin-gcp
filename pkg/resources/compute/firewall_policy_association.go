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

// FirewallPolicyAssociationProvisioner attaches one network firewall policy to
// one VPC network. A policy with rules but no association is inert — the
// association is what puts it in the data path.
//
// Like the rules, an association is not a REST sub-collection but a set of
// verbs on the policy: addAssociation, getAssociation?name=N,
// removeAssociation?name=N. There is nothing to update — an association is a
// (policy, network) pair — so a change replaces it.
//
// Global and regional network firewall policies expose the same three verbs,
// differing only in whether the policy sits under global or regions/{region},
// so both are one provisioner carrying a `regional` flag.
type FirewallPolicyAssociationProvisioner struct {
	*base.BaseResource
	regional bool
}

var _ prov.Provisioner = (*FirewallPolicyAssociationProvisioner)(nil)

// associationPolicyProperty names the owning policy; it is a path component,
// never a body field.
const associationPolicyProperty = "firewallPolicy"

func init() {
	registerPolicyAssociation(NetworkFirewallPolicyAssociationResourceType, false)
	registerPolicyAssociation(RegionNetworkFirewallPolicyAssociationResourceType, true)
}

func registerPolicyAssociation(resourceType string, regional bool) {
	scope := base.ScopeGlobal
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
			return &FirewallPolicyAssociationProvisioner{
				BaseResource: &base.BaseResource{
					Config:          cfg,
					APIConfig:       ComputeAPI,
					OperationConfig: ComputeOperations,
					ResourceConfig: base.ResourceConfig{
						ResourceType: "firewallPolicies",
						Scope:        &base.ScopeConfig{Type: scope},
					},
					NativeIDConfig: ComputeNativeID,
				},
				regional: regional,
			}
		})
}

// scopePath is the segment(s) between the project and the collection.
func (p *FirewallPolicyAssociationProvisioner) scopePath(region string) string {
	if p.regional {
		return "regions/" + region
	}
	return "global"
}

// buildAssociationNativeID composes
// "projects/{p}/{scope}/firewallPolicies/{policy}/associations/{name}", where
// scope is "global" or "regions/{region}".
func (p *FirewallPolicyAssociationProvisioner) buildAssociationNativeID(project, region, policy, name string) string {
	return fmt.Sprintf("projects/%s/%s/firewallPolicies/%s/associations/%s",
		project, p.scopePath(region), policy, name)
}

// parseAssociationNativeID splits the composite id. An association is addressed
// by (policy, name), so both have to survive, and the scope segment must match
// this provisioner's kind — a regional association read against a global URL
// would find nothing, or worse, a same-named global policy.
func (p *FirewallPolicyAssociationProvisioner) parseAssociationNativeID(nativeID string) (project, region, policy, name string, err error) {
	invalid := func() (string, string, string, string, error) {
		return "", "", "", "", fmt.Errorf("invalid firewall policy association native ID: %s", nativeID)
	}
	parts := strings.Split(nativeID, "/")
	// A regional id spells the scope as "regions/{region}"; a global one uses
	// the single segment "global", so it is one shorter.
	off := 0
	if p.regional {
		off = 1
	}
	if len(parts) != 7+off || parts[0] != "projects" {
		return invalid()
	}
	if p.regional {
		if parts[2] != "regions" || parts[3] == "" {
			return invalid()
		}
		region = parts[3]
	} else if parts[2] != "global" {
		return invalid()
	}
	if parts[3+off] != "firewallPolicies" || parts[5+off] != "associations" || parts[6+off] == "" {
		return invalid()
	}
	return parts[1], region, parts[4+off], parts[6+off], nil
}

// associationBody keeps only what the attach verb accepts: the association's own
// name and the network it attaches to.
// associationBody also drops "region": it addresses the policy in the URL and
// the attach verb rejects it as a body field.
func associationBody(props map[string]interface{}) map[string]interface{} {
	body := map[string]interface{}{}
	if name, ok := props["name"].(string); ok && name != "" {
		body["name"] = name
	}
	if target, ok := props["attachmentTarget"].(string); ok && target != "" {
		body["attachmentTarget"] = target
	}
	return body
}

func (p *FirewallPolicyAssociationProvisioner) policyURL(project, region, policy string) string {
	return fmt.Sprintf("%s/projects/%s/%s/firewallPolicies/%s",
		p.APIConfig.BaseURL, project, p.scopePath(region), policy)
}

// regionFor is only meaningful for a regional kind: the region is a path
// component, so it comes from the declared properties, falling back to the
// target's configured region.
func (p *FirewallPolicyAssociationProvisioner) regionFor(props map[string]interface{}, targetConfig json.RawMessage) string {
	if !p.regional {
		return ""
	}
	if region, ok := props["region"].(string); ok && region != "" {
		return region
	}
	if cfg := config.FromTargetConfig(targetConfig); cfg != nil {
		return cfg.Region
	}
	return ""
}

func (p *FirewallPolicyAssociationProvisioner) projectFor(targetConfig json.RawMessage, fallback string) string {
	if cfg := config.FromTargetConfig(targetConfig); cfg != nil && cfg.Project != "" {
		return cfg.Project
	}
	return fallback
}

func (p *FirewallPolicyAssociationProvisioner) issueVerb(
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
		return "", transport.WrapError(err, "firewall policy association verb failed")
	}
	opID := p.OperationConfig.OperationIDExtractor(resp.Body)
	return p.OperationConfig.OperationURLBuilder(
		base.PathContext{Project: project, Region: region}, opID), nil
}

func (p *FirewallPolicyAssociationProvisioner) Create(
	ctx context.Context, request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid properties: %v", err)), nil
	}
	policy, _ := props[associationPolicyProperty].(string)
	name, _ := props["name"].(string)
	target, _ := props["attachmentTarget"].(string)
	if policy == "" || name == "" || target == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"firewallPolicy, name and attachmentTarget are required"), nil
	}
	project := p.projectFor(request.TargetConfig, "")
	if project == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"target project is required"), nil
	}

	region := p.regionFor(props, request.TargetConfig)
	if p.regional && region == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"region is required"), nil
	}

	requestID, verbErr := p.issueVerb(ctx,
		p.policyURL(project, region, policy)+"/addAssociation", associationBody(props), project, region)
	if verbErr != nil {
		return createFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        p.buildAssociationNativeID(project, region, policy, name),
			RequestID:       requestID,
			StatusMessage:   "firewall policy association in progress",
		},
	}, nil
}

func (p *FirewallPolicyAssociationProvisioner) Read(
	ctx context.Context, request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	project, region, policy, name, err := p.parseAssociationNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil
	}
	project = p.projectFor(request.TargetConfig, project)

	client, cErr := transport.NewClient(ctx, p.Config)
	if cErr != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", cErr)
	}
	resp, rErr := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    fmt.Sprintf("%s/getAssociation?name=%s", p.policyURL(project, region, policy), name),
	})
	if rErr != nil {
		wrapped := transport.WrapError(rErr, "failed to read firewall policy association")
		code := transport.ToResourceErrorCode(wrapped.Code)
		// A removed association answers 400 ("does not exist"), not 404, so the
		// generic mapping would report a hard failure and formae would never
		// learn it is gone.
		if code == resource.OperationErrorCodeInvalidRequest &&
			strings.Contains(strings.ToLower(wrapped.Message), "does not exist") {
			code = resource.OperationErrorCodeNotFound
		}
		return &resource.ReadResult{ErrorCode: code}, nil
	}

	props := make(map[string]interface{}, len(resp.Body)+1)
	for k, v := range resp.Body {
		props[k] = v
	}
	// The owning policy is a path component, so put it back for comparison
	// against the declared forma.
	props[associationPolicyProperty] = policy
	if p.regional {
		props["region"] = region
	}

	encoded, mErr := json.Marshal(props)
	if mErr != nil {
		return nil, fmt.Errorf("failed to marshal association properties: %w", mErr)
	}
	return &resource.ReadResult{Properties: string(encoded)}, nil
}

func (p *FirewallPolicyAssociationProvisioner) Update(
	ctx context.Context, request *resource.UpdateRequest,
) (*resource.UpdateResult, error) {
	return updateFailure(resource.OperationErrorCodeNotUpdatable,
		"a firewall policy association is a (policy, network) pair; a change replaces it"), nil
}

func (p *FirewallPolicyAssociationProvisioner) Delete(
	ctx context.Context, request *resource.DeleteRequest,
) (*resource.DeleteResult, error) {
	project, region, policy, name, err := p.parseAssociationNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	project = p.projectFor(request.TargetConfig, project)

	url := fmt.Sprintf("%s/removeAssociation?name=%s", p.policyURL(project, region, policy), name)
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
			StatusMessage:   "firewall policy association removal in progress",
		},
	}, nil
}

// List enumerates one policy's associations. They live inside their policy, so
// discovery has to be told which policy to look in; without that hint an empty
// result is the honest answer.
func (p *FirewallPolicyAssociationProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	policy := ""
	if request.AdditionalProperties != nil {
		policy = request.AdditionalProperties[associationPolicyProperty]
	}
	project := p.projectFor(request.TargetConfig, "")
	if project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}
	region := ""
	if p.regional {
		props := map[string]interface{}{}
		if request.AdditionalProperties != nil {
			if r := request.AdditionalProperties["region"]; r != "" {
				props["region"] = r
			}
		}
		if region = p.regionFor(props, request.TargetConfig); region == "" {
			return &resource.ListResult{NativeIDs: []string{}}, nil
		}
	}

	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}
	// A named policy is the caller telling us where to look. Discovery names
	// none, and an association has no collection URL of its own, so the only way
	// to discover one is to walk the policies first.
	policies := []string{policy}
	if policy == "" {
		policies, err = listComputeCollectionNames(ctx, client,
			fmt.Sprintf("%s/projects/%s/%s/firewallPolicies",
				p.APIConfig.BaseURL, project, p.scopePath(region)),
			"firewall policies")
		if err != nil {
			return nil, err
		}
	}

	nativeIDs := []string{}
	for _, name := range policies {
		resp, rErr := client.SendRequest(ctx, transport.RequestOptions{
			Method: "GET",
			URL:    p.policyURL(project, region, name),
		})
		if rErr != nil {
			if policy != "" {
				wrapped := transport.WrapError(rErr, "failed to list firewall policy associations")
				return nil, fmt.Errorf("%s", wrapped.Message)
			}
			// One unreadable policy must not hide the rest.
			continue
		}
		associations, _ := resp.Body["associations"].([]interface{})
		for _, a := range associations {
			assoc, ok := a.(map[string]interface{})
			if !ok {
				continue
			}
			if assocName, ok := assoc["name"].(string); ok && assocName != "" {
				nativeIDs = append(nativeIDs,
					p.buildAssociationNativeID(project, region, name, assocName))
			}
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// Status routes through the shared read-back so post-create and post-update
// state carries the resource's real properties, not just what was declared.
func (p *FirewallPolicyAssociationProvisioner) Status(
	ctx context.Context, request *resource.StatusRequest,
) (*resource.StatusResult, error) {
	return base.StatusWithRead(ctx, p.BaseResource, p.Read, request)
}
