// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package alloydb implements GCP AlloyDB resources.
package alloydb

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const (
	ClusterResourceType  = "GCP::AlloyDB::Cluster"
	InstanceResourceType = "GCP::AlloyDB::Instance"
	UserResourceType     = "GCP::AlloyDB::User"
	BackupResourceType   = "GCP::AlloyDB::Backup"
)

var alloyDBRegistry *base.ResourceRegistry

func init() {
	alloyDBRegistry = base.NewResourceRegistry(
		AlloyDBAPI, AlloyDBOperations, AlloyDBNativeID)

	// ponytail: Update deferred (as DNS/CloudRun/artifactregistry do) until
	// PATCH is verified live. create/delete are the async paths this proves out.
	err := alloyDBRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: ClusterResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "clusters",
				Scope:         &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam: "clusterId", // id goes in ?clusterId=, not the body
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			ResponseTransformer: clusterResponseTransformer,
		},
		// Instance - nested under a cluster:
		// /projects/{p}/locations/{loc}/clusters/{cluster}/instances/{name}.
		// A cluster without one serves no traffic.
		{
			ResourceType:    InstanceResourceType,
			OperationConfig: AlloyDBInstanceOperations,
			NativeIDConfig:  AlloyDBInstanceNativeID,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "instances",
				Scope:        &base.ScopeConfig{Type: base.ScopeLocationBased},
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "clusters",
					PropertyName:   "cluster",
					RequiresParent: true,
				},
				CreateIDParam: "instanceId",
			},
			// "cluster" identifies the instance's place in the URL, not a body
			// field.
			RequestTransformer: base.DropFields("cluster"),
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			ResponseTransformer: instanceResponseTransformer,
		},
		// User - a database user in a cluster. Unlike the cluster and instance
		// this path is synchronous: users.create returns the User directly.
		// ponytail: update is off — patch takes an updateMask, but the mutable
		// set (password, databaseRoles) needs its own verification pass; a
		// change replaces for now.
		{
			ResourceType:    UserResourceType,
			OperationConfig: AlloyDBUserOperations,
			NativeIDConfig:  AlloyDBUserNativeID,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "users",
				Scope:        &base.ScopeConfig{Type: base.ScopeLocationBased},
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "clusters",
					PropertyName:   "cluster",
					RequiresParent: true,
				},
				CreateIDParam: "userId",
			},
			// "cluster" is a path component, not a body field.
			RequestTransformer: base.DropFields("cluster"),
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			ResponseTransformer: userResponseTransformer,
		},
		// Backup - location-scoped, not nested: it names its cluster in the body
		// rather than sitting under it. Async, like the cluster and instance.
		{
			ResourceType: BackupResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "backups",
				Scope:         &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam: "backupId",
			},
			RequestTransformer: backupRequestTransformer,
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			ResponseTransformer: backupResponseTransformer,
		},
	})
	if err != nil {
		panic(err)
	}

	// Users need a List that walks the clusters; see user_list.go.
	registerUserListOverride()
}
