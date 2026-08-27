// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package sql

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// SQLBackupRunOperations differs from the shared SQL config in one place: a
// backup run's id is server-generated, so the native ID has to come out of the
// create Operation's backupContext rather than from any declared property.
var SQLBackupRunOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   SQLOperations.OperationIDExtractor,
	OperationURLBuilder:    SQLOperations.OperationURLBuilder,
	NativeIDExtractor:      extractBackupRunNativeID,
	OperationStatusChecker: SQLOperations.OperationStatusChecker,
	RetryableError:         SQLOperations.RetryableError,
}

// extractBackupRunNativeID addresses a backup run by the numeric id sqladmin
// assigns it, which get and delete take as their path segment.
//
// The id arrives in two shapes: as backupContext.backupId on the create
// Operation, and as "id" on a get or a list item. Both are strings - Google
// renders int64 as a string - so nothing here parses a number.
func extractBackupRunNativeID(response map[string]interface{}, ctx base.PathContext) string {
	id := backupRunID(response)
	if id == "" || ctx.ParentResource == "" {
		return ""
	}
	return fmt.Sprintf("projects/%s/instances/%s/backupRuns/%s",
		ctx.Project, ctx.ParentResource, id)
}

func backupRunID(response map[string]interface{}) string {
	if id := utils.GetString(response, "id"); id != "" {
		return id
	}
	if backupContext, ok := response["backupContext"].(map[string]interface{}); ok {
		return utils.GetString(backupContext, "backupId")
	}
	return ""
}

// backupRunResponseTransformer drops what sqladmin echoes that addresses the
// backup rather than describing it. "instance" stays: a forma declares it, and
// it is the only place the owning instance appears in a read.
func backupRunResponseTransformer(props map[string]interface{}, _ base.TransformContext) map[string]interface{} {
	out := make(map[string]interface{}, len(props))
	for k, v := range props {
		switch k {
		case "kind", "selfLink", "project":
			continue
		}
		out[k] = v
	}
	return out
}

// backupRunListProvisioner walks the instances for discovery, which names none.
type backupRunListProvisioner struct {
	prov.Provisioner
	cfg *config.Config
}

// registerBackupRunOverrides is called from the package init in resources.go so
// the generic registration is guaranteed to have landed first.
func registerBackupRunOverrides() {
	registry.Register(BackupRunResourceType,
		[]resource.Operation{resource.OperationList},
		func(cfg *config.Config) prov.Provisioner {
			return &backupRunListProvisioner{
				Provisioner: sqlRegistry.CreateProvisioner(cfg, BackupRunResourceType),
				cfg:         cfg,
			}
		})
}

func (p *backupRunListProvisioner) List(
	ctx context.Context, request *resource.ListRequest,
) (*resource.ListResult, error) {
	if request.AdditionalProperties != nil && request.AdditionalProperties["instance"] != "" {
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

	instancesURL := fmt.Sprintf("%s/projects/%s/instances", SQLAPI.BaseURL, cfg.Project)
	resp, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: instancesURL})
	if err != nil {
		wrapped := transport.WrapError(err, "failed to list SQL instances")
		return nil, fmt.Errorf("%s", wrapped.Message)
	}

	nativeIDs := []string{}
	instances, _ := resp.Body["items"].([]interface{})
	for _, raw := range instances {
		inst, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		instName, _ := inst["name"].(string)
		if instName == "" {
			continue
		}
		runsResp, listErr := client.SendRequest(ctx, transport.RequestOptions{
			Method: "GET",
			URL:    fmt.Sprintf("%s/%s/backupRuns", instancesURL, instName),
		})
		if listErr != nil {
			// One unreadable instance must not hide the rest.
			continue
		}
		runs, _ := runsResp.Body["items"].([]interface{})
		for _, rawRun := range runs {
			run, ok := rawRun.(map[string]interface{})
			if !ok {
				continue
			}
			if id := backupRunID(run); id != "" {
				nativeIDs = append(nativeIDs,
					fmt.Sprintf("projects/%s/instances/%s/backupRuns/%s", cfg.Project, instName, id))
			}
		}
	}
	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}
