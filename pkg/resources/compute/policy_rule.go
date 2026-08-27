// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// PolicyRuleProvisioner manages one rule inside a policy. Nothing here can go
// through the generic engine: a rule is not a REST sub-collection but a set of
// verbs on the policy — addRule, getRule?priority=N, patchRule?priority=N,
// removeRule?priority=N — so all of CRUD is hand-written. Status delegates to
// the base, since every verb returns an ordinary global Compute operation.
//
// Network firewall policies and Cloud Armor security policies expose the same
// four verbs over the same shape, differing only in the collection segment, the
// property naming the owning policy, and where GCP's own rules start — so both
// are one provisioner parameterised by policyRuleKind.
type PolicyRuleProvisioner struct {
	*base.BaseResource
	kind policyRuleKind
}

var _ prov.Provisioner = (*PolicyRuleProvisioner)(nil)

// policyRuleKind is everything that differs between the two rule flavours.
type policyRuleKind struct {
	// collection is the policy's URL segment, which is also its method group.
	collection string
	// policyProperty is the schema field naming the owning policy. It is a path
	// component, never part of a rule body.
	policyProperty string
	// priorityFloor is where GCP's own rules start. Rules at or above it are not
	// manageable, so List must not report them as discoverable resources.
	priorityFloor int
	// regional puts the policy under regions/{region} instead of global, and
	// makes "region" a path component of the rule rather than a body field.
	regional bool
	// label appears in status messages and errors.
	label string
}

// impliedRulePriorityFloor is where a network firewall policy's implied rules
// start. A policy is created with several of them (goto_next / deny).
const impliedRulePriorityFloor = 2147483644

// defaultSecurityRulePriority is the one rule Cloud Armor creates for a new
// policy — a catch-all allow that cannot be removed on its own.
const defaultSecurityRulePriority = 2147483647

var (
	firewallPolicyRuleKind = policyRuleKind{
		collection:     "firewallPolicies",
		policyProperty: "firewallPolicy",
		priorityFloor:  impliedRulePriorityFloor,
		label:          "firewall policy rule",
	}
	securityPolicyRuleKind = policyRuleKind{
		collection:     "securityPolicies",
		policyProperty: "securityPolicy",
		priorityFloor:  defaultSecurityRulePriority,
		label:          "security policy rule",
	}
	// The regional verbs are identical, only the policy sits under a region.
	regionSecurityPolicyRuleKind = policyRuleKind{
		collection:     "securityPolicies",
		policyProperty: "securityPolicy",
		priorityFloor:  defaultSecurityRulePriority,
		label:          "regional security policy rule",
		regional:       true,
	}
)

func init() {
	registerPolicyRuleKind(NetworkFirewallPolicyRuleResourceType, firewallPolicyRuleKind)
	registerPolicyRuleKind(SecurityPolicyRuleResourceType, securityPolicyRuleKind)
	registerPolicyRuleKind(RegionSecurityPolicyRuleResourceType, regionSecurityPolicyRuleKind)
}

func registerPolicyRuleKind(resourceType string, kind policyRuleKind) {
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
			return &PolicyRuleProvisioner{
				BaseResource: &base.BaseResource{
					Config:          cfg,
					APIConfig:       ComputeAPI,
					OperationConfig: ComputeOperations,
					ResourceConfig: base.ResourceConfig{
						ResourceType: kind.collection,
						Scope:        &base.ScopeConfig{Type: kind.scopeType()},
					},
					NativeIDConfig: ComputeNativeID,
				},
				kind: kind,
			}
		})
}

func (k policyRuleKind) scopeType() base.ScopeType {
	if k.regional {
		return base.ScopeRegional
	}
	return base.ScopeGlobal
}

// scopePath is the segment(s) between the project and the collection.
func (k policyRuleKind) scopePath(region string) string {
	if k.regional {
		return "regions/" + region
	}
	return "global"
}

// nativeID composes "projects/{p}/{scope}/{collection}/{policy}/rules/{priority}",
// where scope is "global" or "regions/{region}".
func (k policyRuleKind) nativeID(project, region, policy string, priority int) string {
	return fmt.Sprintf("projects/%s/%s/%s/%s/rules/%d",
		project, k.scopePath(region), k.collection, policy, priority)
}

// parseNativeID splits the composite id back into its parts. A rule has no
// identity of its own in the API — it is addressed by (policy, priority) — so
// both have to survive in the native ID.
func (k policyRuleKind) parseNativeID(nativeID string) (project, region, policy string, priority int, err error) {
	invalid := func() (string, string, string, int, error) {
		return "", "", "", 0, fmt.Errorf("invalid %s native ID: %s", k.label, nativeID)
	}
	parts := strings.Split(nativeID, "/")
	// A regional id spells the scope as "regions/{region}" where a global one
	// uses the single segment "global", so everything after the scope sits one
	// index further along.
	off := 0
	if k.regional {
		off = 1
	}
	if len(parts) != 7+off || parts[0] != "projects" {
		return invalid()
	}
	if k.regional {
		if parts[2] != "regions" || parts[3] == "" {
			return invalid()
		}
		region = parts[3]
	} else if parts[2] != "global" {
		return invalid()
	}
	if parts[3+off] != k.collection || parts[5+off] != "rules" {
		return invalid()
	}
	priority, err = strconv.Atoi(parts[6+off])
	if err != nil {
		return "", "", "", 0, fmt.Errorf("invalid priority in native ID %s: %w", nativeID, err)
	}
	return parts[1], region, parts[4+off], priority, nil
}

// body strips the properties that address the rule rather than describe it. The
// owning policy is a path component, and "priority" travels as a query parameter
// on patch, but the API also accepts it in the body on add, so it is kept there.
func (k policyRuleKind) body(props map[string]interface{}, keepPriority bool) map[string]interface{} {
	out := make(map[string]interface{}, len(props))
	for key, v := range props {
		if key == k.policyProperty {
			continue
		}
		if key == "region" && k.regional {
			continue
		}
		if key == "priority" && !keepPriority {
			continue
		}
		out[key] = v
	}
	return out
}

func (p *PolicyRuleProvisioner) policyURL(project, region, policy string) string {
	return fmt.Sprintf("%s/projects/%s/%s/%s/%s",
		p.APIConfig.BaseURL, project, p.kind.scopePath(region), p.kind.collection, policy)
}

// projectFor prefers the project the target is configured with, falling back to
// whatever the native ID carried.
func (p *PolicyRuleProvisioner) projectFor(targetConfig json.RawMessage, fallback string) string {
	if cfg := config.FromTargetConfig(targetConfig, nil /* path context only; this config never authenticates */); cfg != nil && cfg.Project != "" {
		return cfg.Project
	}
	return fallback
}

// regionFor is only meaningful for a regional kind: the region is a path
// component, so it comes from the declared properties, falling back to the
// target's configured region.
func (p *PolicyRuleProvisioner) regionFor(props map[string]interface{}, targetConfig json.RawMessage) string {
	if !p.kind.regional {
		return ""
	}
	if region, ok := props["region"].(string); ok && region != "" {
		return region
	}
	if cfg := config.FromTargetConfig(targetConfig, nil /* path context only; this config never authenticates */); cfg != nil {
		return cfg.Region
	}
	return ""
}

// issueVerb POSTs one of the rule verbs and returns the operation path to poll.
func (p *PolicyRuleProvisioner) issueVerb(
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

func (p *PolicyRuleProvisioner) Create(
	ctx context.Context, request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid properties: %v", err)), nil
	}
	policy, ok := props[p.kind.policyProperty].(string)
	if !ok || policy == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("%s is required", p.kind.policyProperty)), nil
	}
	priority, err := priorityOf(props)
	if err != nil {
		return createFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	project := p.projectFor(request.TargetConfig, "")
	if project == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"target project is required"), nil
	}

	region := p.regionFor(props, request.TargetConfig)
	if p.kind.regional && region == "" {
		return createFailure(resource.OperationErrorCodeInvalidRequest,
			"region is required"), nil
	}

	requestID, verbErr := p.issueVerb(ctx,
		p.policyURL(project, region, policy)+"/addRule", p.kind.body(props, true), project, region)
	if verbErr != nil {
		return createFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        p.kind.nativeID(project, region, policy, priority),
			RequestID:       requestID,
			StatusMessage:   p.kind.label + " creation in progress",
		},
	}, nil
}

func (p *PolicyRuleProvisioner) Read(
	ctx context.Context, request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	project, region, policy, priority, err := p.kind.parseNativeID(request.NativeID)
	if err != nil {
		return &resource.ReadResult{
			ErrorCode: resource.OperationErrorCodeInvalidRequest,
		}, nil
	}
	project = p.projectFor(request.TargetConfig, project)

	client, cErr := transport.NewClient(ctx, p.Config)
	if cErr != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", cErr)
	}
	resp, rErr := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    fmt.Sprintf("%s/getRule?priority=%d", p.policyURL(project, region, policy), priority),
	})
	if rErr != nil {
		wrapped := transport.WrapError(rErr, "failed to read "+p.kind.label)
		code := transport.ToResourceErrorCode(wrapped.Code)
		// A removed rule answers 400 ("no rule with priority"), not 404, so the
		// generic mapping would report a hard failure and formae would never
		// learn the rule is gone.
		if code == resource.OperationErrorCodeInvalidRequest &&
			strings.Contains(strings.ToLower(wrapped.Message), "priority") {
			code = resource.OperationErrorCodeNotFound
		}
		return &resource.ReadResult{ErrorCode: code}, nil
	}

	// The owning policy is a path component, so put it back for comparison
	// against the declared forma.
	props := make(map[string]interface{}, len(resp.Body)+1)
	for k, v := range resp.Body {
		props[k] = v
	}
	props[p.kind.policyProperty] = policy
	if p.kind.regional {
		props["region"] = region
	}

	encoded, mErr := json.Marshal(props)
	if mErr != nil {
		return nil, fmt.Errorf("failed to marshal rule properties: %w", mErr)
	}
	return &resource.ReadResult{Properties: string(encoded)}, nil
}

func (p *PolicyRuleProvisioner) Update(
	ctx context.Context, request *resource.UpdateRequest,
) (*resource.UpdateResult, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(request.DesiredProperties, &props); err != nil {
		return updateFailure(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid properties: %v", err)), nil
	}
	project, region, policy, priority, err := p.kind.parseNativeID(request.NativeID)
	if err != nil {
		return updateFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	project = p.projectFor(request.TargetConfig, project)

	url := fmt.Sprintf("%s/patchRule?priority=%d", p.policyURL(project, region, policy), priority)
	requestID, verbErr := p.issueVerb(ctx, url, p.kind.body(props, true), project, region)
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

func (p *PolicyRuleProvisioner) Delete(
	ctx context.Context, request *resource.DeleteRequest,
) (*resource.DeleteResult, error) {
	project, region, policy, priority, err := p.kind.parseNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	project = p.projectFor(request.TargetConfig, project)

	url := fmt.Sprintf("%s/removeRule?priority=%d", p.policyURL(project, region, policy), priority)
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

// List enumerates the manageable rules of one policy. Rules live inside their
// policy, so discovery needs to be told which policy to look in; without that
// hint there is nothing to enumerate and an empty result is the honest answer.
func (p *PolicyRuleProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	policy := ""
	if request.AdditionalProperties != nil {
		policy = request.AdditionalProperties[p.kind.policyProperty]
	}
	project := p.projectFor(request.TargetConfig, "")
	if project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}
	region := ""
	if p.kind.regional {
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
	// none, and a rule has no collection URL of its own - it lives inside its
	// policy - so the only way to discover one is to walk the policies first.
	policies := []string{policy}
	if policy == "" {
		policies, err = p.listPolicies(ctx, client, project, region)
		if err != nil {
			return nil, err
		}
	}

	nativeIDs := []string{}
	for _, name := range policies {
		rules, err := p.rulesOfPolicy(ctx, client, project, region, name)
		if err != nil {
			// One unreadable policy must not hide the rest.
			if policy != "" {
				return nil, err
			}
			continue
		}
		nativeIDs = append(nativeIDs, rules...)
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// listPolicies names every policy of this kind in the project (and region).
func (p *PolicyRuleProvisioner) listPolicies(
	ctx context.Context, client *transport.Client, project, region string,
) ([]string, error) {
	return listComputeCollectionNames(ctx, client, fmt.Sprintf("%s/projects/%s/%s/%s",
		p.APIConfig.BaseURL, project, p.kind.scopePath(region), p.kind.collection),
		p.kind.label+" policies")
}

// rulesOfPolicy reports the manageable rules of one policy as native IDs.
func (p *PolicyRuleProvisioner) rulesOfPolicy(
	ctx context.Context, client *transport.Client, project, region, policy string,
) ([]string, error) {
	resp, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    p.policyURL(project, region, policy),
	})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to list "+p.kind.label+"s")
		return nil, fmt.Errorf("%s", wrapped.Message)
	}

	rules, _ := resp.Body["rules"].([]interface{})
	nativeIDs := make([]string, 0, len(rules))
	for _, r := range rules {
		ruleMap, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		priority, err := priorityOf(ruleMap)
		if err != nil || priority >= p.kind.priorityFloor {
			continue
		}
		nativeIDs = append(nativeIDs, p.kind.nativeID(project, region, policy, priority))
	}
	return nativeIDs, nil
}

func priorityOf(props map[string]interface{}) (int, error) {
	switch v := props["priority"].(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	case json.Number:
		i, err := v.Int64()
		return int(i), err
	case string:
		return strconv.Atoi(v)
	default:
		return 0, fmt.Errorf("priority is required and must be a number")
	}
}

// Status routes through the shared read-back so post-create and post-update
// state carries the resource's real properties, not just what was declared.
func (p *PolicyRuleProvisioner) Status(
	ctx context.Context, request *resource.StatusRequest,
) (*resource.StatusResult, error) {
	return base.StatusWithRead(ctx, p.BaseResource, p.Read, request)
}
