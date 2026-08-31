// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package sql

import (
	"fmt"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
)

// Resource type constants
const (
	DatabaseInstanceResourceType = "GCP::SQL::DatabaseInstance"
	UserResourceType             = "GCP::SQL::User"
	SslCertResourceType          = "GCP::SQL::SslCert"
	BackupRunResourceType        = "GCP::SQL::BackupRun"
	DatabaseResourceType         = "GCP::SQL::Database"
)

// sqlRegistry is the unified registry for all SQL resources
var sqlRegistry *base.ResourceRegistry

func NewSQLProvisioner(cfg *config.Config, resourceType string) (prov.Provisioner, error) {
	if sqlRegistry == nil {
		return nil, fmt.Errorf("sql registry not initialized")
	}

	_, ok := sqlRegistry.Definitions[resourceType]
	if !ok {
		return nil, fmt.Errorf("no configuration found for resource type: %s", resourceType)
	}

	// Use the registry's provisioner creation
	return sqlRegistry.CreateProvisioner(cfg, resourceType), nil
}

// Wrapper function to adapt response transformer to base.ResponseTransformer interface
func wrapResponseTransformer(responseTransformer func(map[string]interface{}) map[string]interface{}) base.ResponseTransformer {
	if responseTransformer == nil {
		return nil
	}
	return base.ResponseTransformerFunc(func(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
		return responseTransformer(apiResponse)
	})
}

func init() {
	// Create the registry with common SQL API configurations
	sqlRegistry = base.NewResourceRegistry(
		SQLAPI,
		SQLOperations,
		SQLNativeID,
	)

	// Register all SQL resources
	err := sqlRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: DatabaseInstanceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "instances",
				Scope:          &base.ScopeConfig{Type: base.ScopeProjectLevel},
				ParentResource: nil, // Project-scoped resource
				SupportsUpdate: false,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       false,
					FieldName:     "",
					LocationInURL: false,
				},
			},
			RequestTransformer:  nil, // Pass through properties
			ResponseTransformer: wrapResponseTransformer(databaseInstanceResponseTransformer),
		},
		{
			ResourceType: DatabaseResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "databases",
				// A database is nested under its instance:
				// /projects/{p}/instances/{instance}/databases/{name}
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "instances",
					PropertyName:   "instance",
					RequiresParent: true,
				},
				SupportsUpdate: false,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled: false,
				},
			},
			RequestTransformer:  nil, // Pass through properties
			ResponseTransformer: nil,
		},
		{
			// A user is nested under its instance like a database is, but the
			// API addresses it inconsistently: get takes the name as a path
			// segment while delete takes it as a query parameter.
			// user.go overrides delete.
			ResourceType: UserResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "users",
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "instances",
					PropertyName:   "instance",
					RequiresParent: true,
				},
				// ponytail: no update. Every field the schema models is fixed at
				// creation, so a change correctly replaces.
				SupportsUpdate: false,
			},
			RequestTransformer:  base.DropFields("instance"),
			ResponseTransformer: base.ResponseTransformerFunc(userResponseTransformer),
		},
		{
			// A client certificate is addressed by its server-generated
			// sha1Fingerprint - get and delete both take it as the path
			// segment - while a forma declares only commonName. It is also the
			// one sqladmin resource whose create answers with the resource
			// itself rather than only an Operation, hence its own
			// OperationConfig.
			ResourceType:    SslCertResourceType,
			OperationConfig: SQLSslCertOperations,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "sslCerts",
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "instances",
					PropertyName:   "instance",
					RequiresParent: true,
				},
				// A certificate is immutable: sqladmin has no update method for
				// one at all.
				SupportsUpdate: false,
			},
			RequestTransformer:  base.DropFields("instance"),
			ResponseTransformer: base.ResponseTransformerFunc(sslCertResponseTransformer),
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
		},
		{
			// A backup run is addressed by the numeric id sqladmin assigns it,
			// which arrives on the create Operation as backupContext.backupId -
			// hence its own OperationConfig. There is no update method: a
			// backup is a point in time, not a thing you edit.
			ResourceType:    BackupRunResourceType,
			OperationConfig: SQLBackupRunOperations,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "backupRuns",
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "instances",
					PropertyName:   "instance",
					RequiresParent: true,
				},
				SupportsUpdate: false,
			},
			RequestTransformer:  base.DropFields("instance"),
			ResponseTransformer: base.ResponseTransformerFunc(backupRunResponseTransformer),
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
		},
	})

	if err != nil {
		panic(err)
	}

	// Re-register Database behind a provisioner that walks the instances on
	// List. Everything else stays config-driven; only List has to differ,
	// because Cloud SQL has no way to ask for databases across instances. See
	// database_walking_list.go.
	def := sqlRegistry.Definitions[DatabaseResourceType]
	registry.Register(DatabaseResourceType, def.Operations, func(cfg *config.Config) prov.Provisioner {
		return &databaseProvisioner{
			BaseResource: &base.BaseResource{
				Config:              cfg,
				APIConfig:           SQLAPI,
				OperationConfig:     SQLOperations,
				ResourceConfig:      def.ResourceConfig,
				NativeIDConfig:      SQLNativeID,
				RequestTransformer:  def.RequestTransformer,
				ResponseTransformer: def.ResponseTransformer,
			},
		}
	})
	registerUserOverrides()
	registerBackupRunReadOverride()
	registerInstanceWalkingLists()
}

// userResponseTransformer puts back the instance the API leaves in the resource
// path, and drops the fields sqladmin echoes that only address the user. A user
// reports "project" and "instance" of its own, but "project" is the target's
// and would read as a property nobody declared.
func userResponseTransformer(props map[string]interface{}, _ base.TransformContext) map[string]interface{} {
	out := make(map[string]interface{}, len(props))
	for k, v := range props {
		if k == "project" || k == "kind" || k == "etag" {
			continue
		}
		out[k] = v
	}
	return out
}
