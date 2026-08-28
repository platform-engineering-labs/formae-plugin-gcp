// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package storage

import (
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
)

// Resource type constants
const (
	BucketResourceType                     = "GCP::Storage::Bucket"
	AnywhereCacheResourceType              = "GCP::Storage::AnywhereCache"
	BucketAccessControlResourceType        = "GCP::Storage::BucketAccessControl"
	DefaultObjectAccessControlResourceType = "GCP::Storage::DefaultObjectAccessControl"
	ManagedFolderResourceType              = "GCP::Storage::ManagedFolder"
	FolderResourceType                     = "GCP::Storage::Folder"
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
		{
			// A managed folder is an IAM boundary inside a bucket: it lets a
			// policy be attached to a prefix without giving it to the whole
			// bucket. It requires uniform bucket-level access - GCS refuses to
			// create one where per-object ACLs still apply.
			//
			// Its name ends with a slash, which is part of its identity and is
			// escaped in the URL.
			ResourceType: ManagedFolderResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "managedFolders",
				ParentResource: &base.ParentResourceConfig{
					ParentType:         "bucket",
					RequiresParent:     true,
					ParentPathSegments: []string{"b"},
				},
				// A managed folder carries nothing but its name; there is
				// nothing an update could change.
				SupportsUpdate: false,
			},
			RequestTransformer:  base.DropFields("bucket"),
			ResponseTransformer: base.ResponseTransformerFunc(bucketScopedResponseTransformer),
		},
		{
			// A folder is a real directory, available only in a bucket created
			// with a hierarchical namespace. Where a managed folder is an IAM
			// boundary over a prefix, a folder is an actual node - renaming one
			// moves everything beneath it.
			ResourceType: FolderResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "folders",
				ParentResource: &base.ParentResourceConfig{
					ParentType:         "bucket",
					RequiresParent:     true,
					ParentPathSegments: []string{"b"},
				},
				SupportsUpdate: false,
			},
			RequestTransformer:  base.DropFields("bucket"),
			ResponseTransformer: base.ResponseTransformerFunc(bucketScopedResponseTransformer),
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

// bucketScopedResponseTransformer puts back the bucket a folder belongs to and
// drops what GCS echoes that describes the request rather than the resource.
// Both folder types report "bucket" themselves, so unlike most nested resources
// here nothing has to be recovered from the URL.
func bucketScopedResponseTransformer(
	props map[string]interface{}, _ base.TransformContext,
) map[string]interface{} {
	out := make(map[string]interface{}, len(props))
	for k, v := range props {
		switch k {
		case "kind", "selfLink", "metageneration", "id":
			continue
		}
		out[k] = v
	}
	return out
}
