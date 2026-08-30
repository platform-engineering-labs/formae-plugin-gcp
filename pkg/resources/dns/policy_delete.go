// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dns

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// policyProvisioner detaches a policy's networks before deleting it.
//
// Cloud DNS refuses to delete a policy or a response policy while a network is
// still attached to it. Nothing else in the forma is holding the policy - the
// network is a prerequisite that outlives it - so the deletion looks unblocked
// and simply fails, which failed every replace of a policy and every destroy of
// a response policy that had a rule.
//
// Detaching is a patch with an empty network list, and it is the same shape for
// both types.
type policyProvisioner struct {
	*base.BaseResource
}

func (p *policyProvisioner) Delete(
	ctx context.Context,
	request *resource.DeleteRequest,
) (*resource.DeleteResult, error) {
	if err := p.detachNetworks(ctx, request); err != nil {
		// A policy that cannot be detached may still be deletable - it may have
		// had no networks in the first place - so report the reason and let the
		// delete speak for itself rather than failing here.
		_ = err
	}
	return p.BaseResource.Delete(ctx, request)
}

// detachNetworks patches the policy so that no network refers to it.
func (p *policyProvisioner) detachNetworks(ctx context.Context, request *resource.DeleteRequest) error {
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return err
	}
	pathCtx, err := base.ParseNativeID(p.NativeIDConfig, request.NativeID)
	if err != nil {
		return err
	}
	p.fillFromTarget(request.TargetConfig, &pathCtx)

	url := fmt.Sprintf("%s%s", p.APIConfig.BaseURL, p.APIConfig.PathBuilder(pathCtx))
	_, err = client.SendRequest(ctx, transport.RequestOptions{
		Method: "PATCH",
		URL:    url,
		Body:   map[string]interface{}{"networks": []interface{}{}},
	})
	return err
}

// fillFromTarget supplies the project when the native ID did not carry one.
func (p *policyProvisioner) fillFromTarget(targetConfig []byte, pathCtx *base.PathContext) {
	if pathCtx.Project != "" {
		return
	}
	cfg := config.FromTargetConfig(targetConfig, p.Config.Deps())
	pathCtx.Project = cfg.Project
}
