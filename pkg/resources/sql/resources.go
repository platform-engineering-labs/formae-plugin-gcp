// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package sql

import (
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
)

// Resource type constants
const (
	DatabaseInstanceResourceType = "GCP::SQL::DatabaseInstance"
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
	})

	if err != nil {
		panic(err)
	}
}
