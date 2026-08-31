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

// responsePolicyListProvisioner exists because the generic List cannot see a
// response policy at all.
//
// base.extractNativeIDFromItem reads itemMap["name"] and gives up when it is
// empty - before it ever calls the API's own native-ID extractor. A response
// policy names itself "responsePolicyName", so every item was skipped and
// discovery reported nothing: the conformance Discover step timed out with
// "resource did not appear in inventory".
type responsePolicyListProvisioner struct {
	prov.Provisioner
	cfg *config.Config
}

// registerResponsePolicyList is called from the package init in resources.go so
// the generic registration is guaranteed to have landed first.
func registerResponsePolicyList() {
	registry.Register(ResponsePolicyResourceType,
		[]resource.Operation{resource.OperationList},
		func(cfg *config.Config) prov.Provisioner {
			return &responsePolicyListProvisioner{
				Provisioner: dnsRegistry.CreateProvisioner(cfg, ResponsePolicyResourceType),
				cfg:         cfg,
			}
		})
}

func (p *responsePolicyListProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	cfg := config.PathFromTargetConfig(request.TargetConfig)
	if cfg.Project == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, p.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	url := fmt.Sprintf("%s/projects/%s/responsePolicies", DNSAPI.BaseURL, cfg.Project)
	resp, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: url})
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
		if name, _ := policy["responsePolicyName"].(string); name != "" {
			nativeIDs = append(nativeIDs,
				fmt.Sprintf("projects/%s/responsePolicies/%s", cfg.Project, name))
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}
