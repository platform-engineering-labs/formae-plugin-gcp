// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package storage

import (
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
)

// Resource type constants
const (
	BucketResourceType                     = "GCP::Storage::Bucket"
	AnywhereCacheResourceType              = "GCP::Storage::AnywhereCache"
	BucketAccessControlResourceType        = "GCP::Storage::BucketAccessControl"
	DefaultObjectAccessControlResourceType = "GCP::Storage::DefaultObjectAccessControl"
	ObjectAccessControlResourceType        = "GCP::Storage::ObjectAccessControl"
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

// objectScopedRequestTransformer handles the special case of object-scoped resources
// which need both bucket and object in the parent resource path
// It creates a synthetic "bucketObject" property that combines bucket and object
func objectScopedRequestTransformer(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	// Extract bucket and object
	bucket, bucketOk := props["bucket"].(string)
	object, objectOk := props["object"].(string)

	// Create a copy of props for transformation
	body := make(map[string]interface{})
	for k, v := range props {
		// Skip bucket and object (they'll be in the URL)
		if k == "bucket" || k == "object" {
			continue
		}
		body[k] = v
	}

	// Add synthetic combined property if both exist
	if bucketOk && objectOk {
		body["bucketObject"] = fmt.Sprintf("%s/%s", bucket, object)
	}

	return body, nil
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
		// NOTE: ObjectAccessControlResourceType requires special handling for object-scoped resources
		// The base package currently doesn't support resources that need TWO parent properties (bucket + object).
		// This resource type is commented out pending enhancement to base package's parent extraction mechanism.
		// {
		// 	ResourceType: ObjectAccessControlResourceType,
		// 	ResourceConfig: base.ResourceConfig{
		// 		ResourceType: "acl",
		// 		Scope:        nil,
		// 		ParentResource: &base.ParentResourceConfig{
		// 			ParentType:         "bucket", // Need both bucket AND object
		// 			RequiresParent:     true,
		// 			ParentPathSegments: []string{"b", "o"},
		// 		},
		// 		SupportsUpdate: true,
		// 		OptimisticLocking: &base.OptimisticLockingConfig{
		// 			Enabled:       false,
		// 			FieldName:     "",
		// 			LocationInURL: false,
		// 		},
		// 	},
		// 	RequestTransformer:  base.RequestTransformerFunc(objectScopedRequestTransformer),
		// 	ResponseTransformer: nil,
		// },
	})

	if err != nil {
		panic(err)
	}
}
