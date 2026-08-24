// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package artifactregistry

import (
	"context"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// ruleListProvisioner overrides List for Artifact Registry rules and delegates
// everything else to the generic provisioner.
//
// A rule lives under a repository, and discovery lists with no properties, so it
// can name no repository to look in. Artifact Registry has no wildcard for
// either segment - "repositories/-" answers "Repository does not exist" - so the
// only way to discover a rule is to walk the repositories first.
type ruleListProvisioner struct {
	prov.Provisioner
	cfg *config.Config
}

func init() {
	// Registering again replaces only the List entry; create, read, update and
	// delete keep the generic implementation registered above.
	registry.Register(RuleResourceType,
		[]resource.Operation{resource.OperationList},
		func(cfg *config.Config) prov.Provisioner {
			return &ruleListProvisioner{
				Provisioner: artifactRegistryRegistry.CreateProvisioner(cfg, RuleResourceType),
				cfg:         cfg,
			}
		})
}

func (p *ruleListProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	// A named repository is the caller telling us where to look; the generic
	// implementation already handles that.
	if request.AdditionalProperties != nil && request.AdditionalProperties["repository"] != "" {
		return p.Provisioner.List(ctx, request)
	}

	cfg := config.FromTargetConfig(request.TargetConfig)
	project, location := "", ""
	if cfg != nil {
		project = cfg.Project
		location = cfg.Location
		if location == "" {
			location = cfg.Region
		}
	}
	if project == "" || location == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	client, err := transport.NewClient(ctx, p.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}
	base := fmt.Sprintf("%s/projects/%s/locations/%s/repositories",
		ArtifactRegistryAPI.BaseURL, project, location)

	resp, rErr := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: base})
	if rErr != nil {
		wrapped := transport.WrapError(rErr, "failed to list artifact registry repositories")
		return nil, fmt.Errorf("%s", wrapped.Message)
	}

	nativeIDs := []string{}
	repositories, _ := resp.Body["repositories"].([]interface{})
	for _, raw := range repositories {
		repo, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		fullName, _ := repo["name"].(string)
		if fullName == "" {
			continue
		}
		short := fullName[strings.LastIndex(fullName, "/")+1:]
		rulesResp, listErr := client.SendRequest(ctx, transport.RequestOptions{
			Method: "GET",
			URL:    fmt.Sprintf("%s/%s/rules", base, short),
		})
		if listErr != nil {
			// One unreadable repository must not hide the rest.
			continue
		}
		rules, _ := rulesResp.Body["rules"].([]interface{})
		for _, rawRule := range rules {
			rule, ok := rawRule.(map[string]interface{})
			if !ok {
				continue
			}
			// A rule reports its full path, which is already the native ID shape.
			if name, _ := rule["name"].(string); strings.Contains(name, "/rules/") {
				nativeIDs = append(nativeIDs, name)
			}
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}
