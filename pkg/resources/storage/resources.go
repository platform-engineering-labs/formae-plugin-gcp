// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package storage

import (
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// Resource type constants
const (
	BucketResourceType                     = "GCP::Storage::Bucket"
	AnywhereCacheResourceType              = "GCP::Storage::AnywhereCache"
	BucketAccessControlResourceType        = "GCP::Storage::BucketAccessControl"
	DefaultObjectAccessControlResourceType = "GCP::Storage::DefaultObjectAccessControl"
	ObjectAccessControlResourceType        = "GCP::Storage::ObjectAccessControl"
	NotificationResourceType               = "GCP::Storage::Notification"
)

// storageRegistry is the unified registry for all Storage resources
var storageRegistry *base.ResourceRegistry

func NewStorageProvisioner(cfg *config.Config, resourceType string) (prov.Provisioner, error) {
	if storageRegistry == nil {
		return nil, fmt.Errorf("storage registry not initialized")
	}

	_, ok := storageRegistry.Definitions[resourceType]
	if !ok {
		return nil, fmt.Errorf("no configuration found for resource type: %s", resourceType)
	}

	// Use the registry's provisioner creation
	return storageRegistry.CreateProvisioner(cfg, resourceType), nil
}

// Wrapper functions to adapt body builders to base.RequestTransformer interface
func wrapBodyBuilder(bodyBuilder func(map[string]interface{}) (map[string]interface{}, error)) base.RequestTransformer {
	if bodyBuilder == nil {
		return nil
	}
	return base.RequestTransformerFunc(func(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
		return bodyBuilder(props)
	})
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
	// Create the registry with common Storage API configurations
	storageRegistry = base.NewResourceRegistry(
		StorageAPI,
		StorageOperations,
		StorageNativeID,
	)

	// Register all Storage resources
	err := storageRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: BucketResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "b",
				Scope:          nil, // Buckets don't use scope (they're top-level)
				ParentResource: nil, // No parent
				SupportsUpdate: true,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       false, // Bucket updates don't require locking
					FieldName:     "",
					LocationInURL: false,
				},
			},
			RequestTransformer:  wrapBodyBuilder(bucketBodyBuilder),
			ResponseTransformer: wrapResponseTransformer(bucketResponseTransformer),
		},
		{
			ResourceType: AnywhereCacheResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "anywhereCaches",
				Scope:        nil,
				ParentResource: &base.ParentResourceConfig{
					ParentType:         "bucket", // Property name in props
					RequiresParent:     true,
					ParentPathSegments: []string{"b"}, // Parent is bucket
				},
				SupportsUpdate: true,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       false,
					FieldName:     "",
					LocationInURL: false,
				},
			},
			RequestTransformer:  wrapBodyBuilder(bucketScopedBodyBuilder),
			ResponseTransformer: nil,
		},
		{
			// Bucket notification - publishes object change events to a Pub/Sub
			// topic. The id is server-assigned, so a forma does not name one.
			ResourceType: NotificationResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "notificationConfigs",
				Scope:        nil,
				ParentResource: &base.ParentResourceConfig{
					ParentType:         "bucket", // property name in props
					RequiresParent:     true,
					ParentPathSegments: []string{"b"},
				},
				SupportsUpdate: false, // immutable: a change replaces
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			RequestTransformer:  base.RequestTransformerFunc(notificationRequest),
			ResponseTransformer: base.ResponseTransformerFunc(notificationResponse),
		},
		{
			ResourceType: BucketAccessControlResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "acl",
				Scope:        nil,
				ParentResource: &base.ParentResourceConfig{
					ParentType:         "bucket", // Property name in props
					RequiresParent:     true,
					ParentPathSegments: []string{"b"}, // Parent is bucket
				},
				SupportsUpdate: true,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       false,
					FieldName:     "",
					LocationInURL: false,
				},
			},
			RequestTransformer:  wrapBodyBuilder(aclBodyBuilder),
			ResponseTransformer: nil,
		},
		{
			ResourceType: DefaultObjectAccessControlResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "defaultObjectAcl",
				Scope:        nil,
				ParentResource: &base.ParentResourceConfig{
					ParentType:         "bucket", // Property name in props
					RequiresParent:     true,
					ParentPathSegments: []string{"b"}, // Parent is bucket
				},
				SupportsUpdate: true,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       false,
					FieldName:     "",
					LocationInURL: false,
				},
			},
			RequestTransformer:  wrapBodyBuilder(aclBodyBuilder),
			ResponseTransformer: nil,
		},
		{
			// An ACL entry on one object. It hangs off a bucket AND an object,
			// which is why this was commented out: nothing could carry two
			// parent properties. ParentResourceConfig.SecondPropertyName now
			// can, joining them as "{bucket}/{object}" - the form the Storage
			// path builder and native ID already expected.
			ResourceType: ObjectAccessControlResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "acl",
				Scope:        nil,
				ParentResource: &base.ParentResourceConfig{
					ParentType:         "bucket",
					PropertyName:       "bucket",
					SecondPropertyName: "object",
					RequiresParent:     true,
					ParentPathSegments: []string{"b", "o"},
				},
				SupportsUpdate: true,
			},
			RequestTransformer:  wrapBodyBuilder(objectAclBodyBuilder),
			ResponseTransformer: nil,
		},
	})

	if err != nil {
		panic(err)
	}

	// Re-register the two ACL types behind a provisioner that walks the buckets
	// on List. Everything else stays config-driven; only List has to differ,
	// because Cloud Storage has no endpoint spanning buckets. See
	// acl_walking_list.go.
	for _, resourceType := range []string{
		BucketAccessControlResourceType,
		DefaultObjectAccessControlResourceType,
	} {
		rt := resourceType
		def := storageRegistry.Definitions[rt]
		registry.Register(rt, def.Operations, func(cfg *config.Config) prov.Provisioner {
			return &aclProvisioner{
				BaseResource: &base.BaseResource{
					Config:              cfg,
					APIConfig:           StorageAPI,
					OperationConfig:     StorageOperations,
					ResourceConfig:      def.ResourceConfig,
					NativeIDConfig:      StorageNativeID,
					RequestTransformer:  def.RequestTransformer,
					ResponseTransformer: def.ResponseTransformer,
				},
			}
		})
	}

	// An object ACL needs a walk of its own: it hangs off a bucket *and* an
	// object, so the bucket walk above stops one level short of it. See
	// object_acl_walking_list.go.
	oacDef := storageRegistry.Definitions[ObjectAccessControlResourceType]
	registry.Register(ObjectAccessControlResourceType, oacDef.Operations,
		func(cfg *config.Config) prov.Provisioner {
			return &objectAclProvisioner{
				BaseResource: &base.BaseResource{
					Config:              cfg,
					APIConfig:           StorageAPI,
					OperationConfig:     StorageOperations,
					ResourceConfig:      oacDef.ResourceConfig,
					NativeIDConfig:      StorageNativeID,
					RequestTransformer:  oacDef.RequestTransformer,
					ResponseTransformer: oacDef.ResponseTransformer,
				},
			}
		})
}
