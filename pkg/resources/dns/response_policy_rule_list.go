// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dns

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// A rule lives under a response policy, and discovery lists with no properties,
// so it can name no policy to look in. Cloud DNS has no wildcard for the
// segment, so the only way to discover a rule is to walk the response policies
// first.
type responsePolicyRuleListProvisioner struct {
	prov.Provisioner
	cfg *config.Config
}

// registerResponsePolicyRuleList replaces only the List entry; create, read,
// update and delete keep the generic implementation. It is called from the
// package init in resources.go rather than from an init of its own, so it
// cannot matter whether this file sorts before or after resources.go - the
// generic registration must land first, or there would be no provisioner to
// wrap.
func registerResponsePolicyRuleList() {
	registry.Register(ResponsePolicyRuleResourceType,
		[]resource.Operation{resource.OperationList},
		func(cfg *config.Config) prov.Provisioner {
			return &responsePolicyRuleListProvisioner{
				Provisioner: dnsRegistry.CreateProvisioner(cfg, ResponsePolicyRuleResourceType),
				cfg:         cfg,
			}
		})
}

func (p *responsePolicyRuleListProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	// A named policy is the caller telling us where to look.
	if request.AdditionalProperties != nil && request.AdditionalProperties["responsePolicy"] != "" {
		return p.Provisioner.List(ctx, request)
	}

	cfg := config.PathFromTargetConfig(request.TargetConfig)
	if cfg.Project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, p.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	policiesURL := fmt.Sprintf("%s/projects/%s/responsePolicies", DNSAPI.BaseURL, cfg.Project)
	resp, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: policiesURL})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to list DNS response policies")
		return nil, fmt.Errorf("%s", wrapped.Message)
	}

	nativeIDs := []string{}
	policies, _ := resp.Body["responsePolicies"].([]interface{})
	for _, raw := range policies {
		policy, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		// A response policy names itself "responsePolicyName", not "name".
		policyName, _ := policy["responsePolicyName"].(string)
		if policyName == "" {
			continue
		}
		rulesResp, listErr := client.SendRequest(ctx, transport.RequestOptions{
			Method: "GET",
			URL:    fmt.Sprintf("%s/%s/rules", policiesURL, policyName),
		})
		if listErr != nil {
			// One unreadable policy must not hide the rest.
			continue
		}
		// The collection is "rules" in the path but "responsePolicyRules" in
		// the response.
		rules, _ := rulesResp.Body["responsePolicyRules"].([]interface{})
		for _, rawRule := range rules {
			rule, ok := rawRule.(map[string]interface{})
			if !ok {
				continue
			}
			if ruleName, _ := rule["ruleName"].(string); ruleName != "" {
				nativeIDs = append(nativeIDs, fmt.Sprintf(
					"projects/%s/responsePolicies/%s/rules/%s", cfg.Project, policyName, ruleName))
			}
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}
