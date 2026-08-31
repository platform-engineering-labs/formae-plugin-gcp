// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package logging

import (
	"context"
	"encoding/json"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// logBucketDeleteRequested is what Cloud Logging leaves behind instead of
// removing a bucket. A deleted bucket sits in this state for seven days so it
// can be undeleted, and a get answers 200 with it rather than 404.
const logBucketDeleteRequested = "DELETE_REQUESTED"

// logBucketResponseTransformer puts back the location the API leaves in the
// path and shortens the name, so the read state matches what a forma declares.
func logBucketResponseTransformer(
	props map[string]interface{}, _ base.TransformContext,
) map[string]interface{} {
	out := make(map[string]interface{}, len(props)+1)
	for k, v := range props {
		out[k] = v
	}
	// projects/{p}/locations/{loc}/buckets/{id}
	if ctx, err := parseLoggingBucketNativeID(utils.GetString(props, "name")); err == nil {
		out["location"] = ctx.Location
		out["name"] = ctx.ResourceName
	}
	return out
}

// logBucketProvisioner reports a bucket awaiting deletion as gone.
//
// Cloud Logging does not remove a deleted bucket: it moves to DELETE_REQUESTED
// and stays for seven days so it can be undeleted. The generic Read would
// report it as present for a week, so an out-of-band delete would never leave
// inventory and discovery would keep offering buckets that are on their way
// out.
type logBucketProvisioner struct {
	prov.Provisioner
}

// registerLogBucketOverrides is called from the package init in resources.go so
// the generic registration is guaranteed to have landed first.
func registerLogBucketOverrides() {
	registry.Register(LogBucketResourceType,
		[]resource.Operation{resource.OperationRead},
		func(cfg *config.Config) prov.Provisioner {
			return &logBucketProvisioner{
				Provisioner: loggingRegistry.CreateProvisioner(cfg, LogBucketResourceType),
			}
		})
}

func (p *logBucketProvisioner) Read(
	ctx context.Context, request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	result, err := p.Provisioner.Read(ctx, request)
	if err != nil || result == nil || result.ErrorCode != "" || result.Properties == "" {
		return result, err
	}

	var props map[string]interface{}
	if unmarshalErr := json.Unmarshal([]byte(result.Properties), &props); unmarshalErr != nil {
		return result, nil
	}
	if utils.GetString(props, "lifecycleState") == logBucketDeleteRequested {
		return &resource.ReadResult{
			ResourceType: request.ResourceType,
			ErrorCode:    resource.OperationErrorCodeNotFound,
		}, nil
	}
	return result, nil
}
