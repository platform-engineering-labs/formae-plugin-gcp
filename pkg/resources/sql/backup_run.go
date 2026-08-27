// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package sql

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
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

// backupRunDeletedStatus is what Cloud SQL leaves behind instead of removing a
// backup run: the record survives its own deletion as a tombstone, and a get
// answers 200 with this status rather than 404.
const backupRunDeletedStatus = "DELETED"

// liveBackupRunID is backupRunID with tombstones filtered out, for listing.
// Returning "" tells the instance walker to skip the item, which keeps deleted
// backups from being discovered as unmanaged resources.
func liveBackupRunID(item map[string]interface{}) string {
	if utils.GetString(item, "status") == backupRunDeletedStatus {
		return ""
	}
	return backupRunID(item)
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

// backupRunProvisioner exists for one reason: a deleted backup run does not go
// away. Cloud SQL keeps the record and answers a get with 200 and
// status "DELETED", so the generic Read reports the resource as still present
// and formae never retires it from inventory - the conformance OOB-delete step
// times out waiting for it to disappear.
type backupRunProvisioner struct {
	prov.Provisioner
}

// registerBackupRunReadOverride is called from the package init in resources.go
// so the generic registration is guaranteed to have landed first.
func registerBackupRunReadOverride() {
	registry.Register(BackupRunResourceType,
		[]resource.Operation{resource.OperationRead},
		func(cfg *config.Config) prov.Provisioner {
			return &backupRunProvisioner{
				Provisioner: sqlRegistry.CreateProvisioner(cfg, BackupRunResourceType),
			}
		})
}

func (p *backupRunProvisioner) Read(
	ctx context.Context, request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	result, err := p.Provisioner.Read(ctx, request)
	if err != nil || result == nil || result.ErrorCode != "" {
		return result, err
	}

	var props map[string]interface{}
	if err := json.Unmarshal([]byte(result.Properties), &props); err != nil {
		return result, nil
	}
	if utils.GetString(props, "status") == backupRunDeletedStatus {
		return &resource.ReadResult{
			ResourceType: request.ResourceType,
			ErrorCode:    resource.OperationErrorCodeNotFound,
		}, nil
	}
	return result, nil
}
