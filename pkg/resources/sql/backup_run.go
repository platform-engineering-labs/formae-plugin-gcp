// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package sql

import (
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
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
