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
type FirewallPolicyAssociationProvisioner struct {
	*base.BaseResource
}

var _ prov.Provisioner = (*FirewallPolicyAssociationProvisioner)(nil)

// associationPolicyProperty names the owning policy; it is a path component,
// never a body field.
const associationPolicyProperty = "firewallPolicy"

func init() {
	registry.Register(NetworkFirewallPolicyAssociationResourceType,
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
						Scope:        &base.ScopeConfig{Type: base.ScopeGlobal},
					},
					NativeIDConfig: ComputeNativeID,
				},
			}
		})
}

// buildAssociationNativeID composes
// "projects/{p}/global/firewallPolicies/{policy}/associations/{name}".
func buildAssociationNativeID(project, policy, name string) string {
	return fmt.Sprintf("projects/%s/global/firewallPolicies/%s/associations/%s",
		project, policy, name)
}

// parseAssociationNativeID splits the composite id. An association is addressed
// by (policy, name), so both have to survive.
func parseAssociationNativeID(nativeID string) (project, policy, name string, err error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 7 || parts[0] != "projects" || parts[2] != "global" ||
		parts[3] != "firewallPolicies" || parts[5] != "associations" || parts[6] == "" {
		return "", "", "", fmt.Errorf("invalid firewall policy association native ID: %s", nativeID)
	}
	return parts[1], parts[4], parts[6], nil
}

// associationBody keeps only what the attach verb accepts: the association's own
// name and the network it attaches to.
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

func (p *FirewallPolicyAssociationProvisioner) policyURL(project, policy string) string {
	return fmt.Sprintf("%s/projects/%s/global/firewallPolicies/%s",
		p.APIConfig.BaseURL, project, policy)
}

func (p *FirewallPolicyAssociationProvisioner) projectFor(targetConfig json.RawMessage, fallback string) string {
	if cfg := config.FromTargetConfig(targetConfig); cfg != nil && cfg.Project != "" {
		return cfg.Project
	}
	return fallback
}

func (p *FirewallPolicyAssociationProvisioner) issueVerb(
	ctx context.Context, url string, body map[string]interface{}, project string,
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
	return p.OperationConfig.OperationURLBuilder(base.PathContext{Project: project}, opID), nil
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

	requestID, verbErr := p.issueVerb(ctx,
		p.policyURL(project, policy)+"/addAssociation", associationBody(props), project)
	if verbErr != nil {
		return createFailure(transport.ToResourceErrorCode(verbErr.Code), verbErr.Message), nil
	}
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        buildAssociationNativeID(project, policy, name),
			RequestID:       requestID,
			StatusMessage:   "firewall policy association in progress",
		},
	}, nil
}

func (p *FirewallPolicyAssociationProvisioner) Read(
	ctx context.Context, request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	project, policy, name, err := parseAssociationNativeID(request.NativeID)
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
		URL:    fmt.Sprintf("%s/getAssociation?name=%s", p.policyURL(project, policy), name),
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
	project, policy, name, err := parseAssociationNativeID(request.NativeID)
	if err != nil {
		return deleteFailure(resource.OperationErrorCodeInvalidRequest, err.Error()), nil
	}
	project = p.projectFor(request.TargetConfig, project)

	url := fmt.Sprintf("%s/removeAssociation?name=%s", p.policyURL(project, policy), name)
	requestID, verbErr := p.issueVerb(ctx, url, nil, project)
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
	if policy == "" || project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}
	resp, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    p.policyURL(project, policy),
	})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to list firewall policy associations")
		return nil, fmt.Errorf("%s", wrapped.Message)
	}

	associations, _ := resp.Body["associations"].([]interface{})
	nativeIDs := make([]string, 0, len(associations))
	for _, a := range associations {
		assoc, ok := a.(map[string]interface{})
		if !ok {
			continue
		}
		if name, ok := assoc["name"].(string); ok && name != "" {
			nativeIDs = append(nativeIDs, buildAssociationNativeID(project, policy, name))
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}
