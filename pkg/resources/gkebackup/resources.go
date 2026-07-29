// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package gkebackup implements GCP Backup for GKE resources.
package gkebackup

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const BackupPlanResourceType = "GCP::GKEBackup::BackupPlan"

var gkeBackupRegistry *base.ResourceRegistry

func init() {
	gkeBackupRegistry = base.NewResourceRegistry(
		GKEBackupAPI, GKEBackupOperations, GKEBackupNativeID)

	// ponytail: Update deferred (as artifactregistry/DNS/scheduler do) until
	// PATCH is verified live. create/delete are the async paths this proves out.
	err := gkeBackupRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: BackupPlanResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "backupPlans",
				Scope:         &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam: "backupPlanId", // id goes in ?backupPlanId=, not the body
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
	})
	if err != nil {
		panic(err)
	}
}
