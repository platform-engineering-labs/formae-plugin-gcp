// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package filestore implements GCP Cloud Filestore resources.
package filestore

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const (
	InstanceResourceType = "GCP::Filestore::Instance"
	BackupResourceType   = "GCP::Filestore::Backup"
	SnapshotResourceType = "GCP::Filestore::Snapshot"
)

func lifecycleOps() []resource.Operation {
	return []resource.Operation{
		resource.OperationCreate,
		resource.OperationRead,
		resource.OperationDelete,
		resource.OperationList,
		resource.OperationCheckStatus,
	}
}

var filestoreRegistry *base.ResourceRegistry

func init() {
	filestoreRegistry = base.NewResourceRegistry(
		FilestoreAPI, FilestoreOperations, FilestoreNativeID)

	// ponytail: Update deferred (as artifactregistry/DNS/scheduler do) until
	// PATCH is verified live. create/delete are the async paths this batch
	// proves out.
	err := filestoreRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: InstanceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "instances",
				Scope:         &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam: "instanceId", // id goes in ?instanceId=, not the body
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			RequestTransformer:  base.RequestTransformerFunc(dropFilestorePathFields),
			ResponseTransformer: locationResponseTransformer("instances"),
		},
		{
			// A backup is a regional copy of an instance's file share, and
			// outlives the instance it came from. "is not a valid location for
			// backup creation, it should be a valid GCP region" - so its
			// location is a region even when the instance it names is zonal.
			ResourceType: BackupResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "backups",
				Scope:         &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam: "backupId",
			},
			Operations:          lifecycleOps(),
			RequestTransformer:  base.RequestTransformerFunc(backupRequest),
			ResponseTransformer: base.ResponseTransformerFunc(backupResponse),
		},
		{
			// A snapshot lives inside its instance, in the same zone.
			ResourceType: SnapshotResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "snapshots",
				Scope:        &base.ScopeConfig{Type: base.ScopeLocationBased},
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "instances",
					PropertyName:   "instance",
					RequiresParent: true,
				},
				CreateIDParam: "snapshotId",
			},
			Operations:          lifecycleOps(),
			RequestTransformer:  base.RequestTransformerFunc(dropFilestorePathFields),
			ResponseTransformer: base.ResponseTransformerFunc(snapshotResponseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}

	// Re-register the snapshot behind a provisioner that walks the instances on
	// List. Everything else stays config-driven; only List has to differ,
	// because Filestore has no wildcard in the instance position.
	registry.Register(SnapshotResourceType, lifecycleOps(), func(cfg *config.Config) prov.Provisioner {
		def := filestoreRegistry.Definitions[SnapshotResourceType]
		return &snapshotProvisioner{
			BaseResource: &base.BaseResource{
				Config:              cfg,
				APIConfig:           FilestoreAPI,
				OperationConfig:     FilestoreOperations,
				ResourceConfig:      def.ResourceConfig,
				NativeIDConfig:      FilestoreNativeID,
				RequestTransformer:  def.RequestTransformer,
				ResponseTransformer: def.ResponseTransformer,
			},
		}
	})
}
