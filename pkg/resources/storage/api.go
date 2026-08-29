// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package storage

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// StorageAPI defines the API configuration for GCP Storage API
var StorageAPI = base.APIConfig{
	BaseURL:     "https://storage.googleapis.com/storage/v1",
	APIVersion:  "v1",
	PathBuilder: storagePathBuilder,
}

// StorageOperations defines how operations work in the Storage API
var StorageOperations = base.OperationConfig{
	Synchronous: true, // Storage operations are synchronous

	// Storage doesn't have async operations, so these are no-ops
	OperationIDExtractor:   func(response map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(ctx base.PathContext, operationID string) string { return "" },
	NativeIDExtractor:      extractStorageNativeID,
	OperationStatusChecker: func(response map[string]interface{}) (bool, error) { return true, nil },
}

// StorageNativeID defines native ID format for Storage resources
var StorageNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseStorageNativeID,
}

// storagePathBuilder builds Storage API paths
// Storage uses a hierarchical structure with bucket scoping
func storagePathBuilder(ctx base.PathContext) string {
	// Bucket resource
	if ctx.ResourceType == "b" {
		if ctx.ResourceName != "" {
			return fmt.Sprintf("/b/%s", ctx.ResourceName)
		}
		// Bucket creation requires project query parameter
		return fmt.Sprintf("/b?project=%s", ctx.Project)
	}

	// Project-scoped resource (e.g., hmacKeys)
	// ParentResource will be empty for project-scoped
	if ctx.ParentResource == "" {
		if ctx.ResourceName != "" {
			return fmt.Sprintf("/projects/%s/%s/%s", ctx.Project, ctx.ResourceType, ctx.ResourceName)
		}
		return fmt.Sprintf("/projects/%s/%s", ctx.Project, ctx.ResourceType)
	}

	// For bucket-scoped and object-scoped resources,
	// ParentResource contains the bucket name (or "bucket/object" for object-scoped)
	bucketInfo := ctx.ParentResource

	// Check if it's object-scoped (contains "/")
	if strings.Contains(bucketInfo, "/") {
		// Object-scoped: "bucket/object"
		parts := strings.SplitN(bucketInfo, "/", 2)
		if len(parts) == 2 {
			bucketName := parts[0]
			// An object name may contain slashes - "conformance/acl-target.txt"
			// is one object, not a folder and a file - and Cloud Storage wants
			// it percent-encoded in the path. Unescaped, the request addresses a
			// path that does not exist and answers 404.
			objectName := url.PathEscape(parts[1])
			if ctx.ResourceName != "" {
				return fmt.Sprintf("/b/%s/o/%s/%s/%s", bucketName, objectName, ctx.ResourceType, ctx.ResourceName)
			}
			return fmt.Sprintf("/b/%s/o/%s/%s", bucketName, objectName, ctx.ResourceType)
		}
	}

	// Bucket-scoped
	bucketName := bucketInfo
	if ctx.ResourceName != "" {
		return fmt.Sprintf("/b/%s/%s/%s", bucketName, ctx.ResourceType, ctx.ResourceName)
	}
	return fmt.Sprintf("/b/%s/%s", bucketName, ctx.ResourceType)
}

// extractStorageNativeID extracts the native ID (full path) from Storage API response
func extractStorageNativeID(response map[string]interface{}, ctx base.PathContext) string {
	// Extract the resource name from the response
	resourceName := ""
	if accessId, ok := response["accessId"].(string); ok {
		resourceName = accessId
	} else if entity, ok := response["entity"].(string); ok {
		resourceName = entity
	} else if anywhereCacheId, ok := response["anywhereCacheId"].(string); ok {
		resourceName = anywhereCacheId
	} else if name, ok := response["name"].(string); ok {
		resourceName = name
	} else if id, ok := response["id"].(string); ok {
		resourceName = id
	}

	// Build full path native ID based on resource type and parent
	return buildStorageNativeID(resourceName, ctx)
}

// buildStorageNativeID constructs a full path native ID for a Storage resource
func buildStorageNativeID(resourceName string, ctx base.PathContext) string {
	// Bucket: use simple name (buckets are top-level)
	if ctx.ResourceType == "b" {
		return resourceName
	}

	// Project-scoped resource
	if ctx.ParentResource == "" {
		return fmt.Sprintf("projects/%s/%s/%s", ctx.Project, ctx.ResourceType, resourceName)
	}

	// Check if object-scoped (ParentResource contains "/")
	if strings.Contains(ctx.ParentResource, "/") {
		// Object-scoped: "bucket/object"
		parts := strings.SplitN(ctx.ParentResource, "/", 2)
		if len(parts) == 2 {
			return fmt.Sprintf("b/%s/o/%s/%s/%s", parts[0], parts[1], ctx.ResourceType, resourceName)
		}
	}

	// Bucket-scoped
	bucketName := ctx.ParentResource
	return fmt.Sprintf("b/%s/%s/%s", bucketName, ctx.ResourceType, resourceName)
}

// parseStorageNativeID parses a full path native ID into PathContext
func parseStorageNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	ctx := base.PathContext{}

	// Simple bucket name (no slashes)
	if !strings.Contains(nativeID, "/") {
		ctx.ResourceType = "b"
		ctx.ResourceName = nativeID
		return ctx, nil
	}

	// Project-scoped: projects/{project}/{resourceType}/{name}
	if len(parts) >= 4 && parts[0] == "projects" {
		ctx.Project = parts[1]
		ctx.ResourceType = parts[2]
		ctx.ResourceName = parts[3]
		ctx.ParentResource = ""
		return ctx, nil
	}

	// Bucket-scoped: b/{bucket}/{resourceType}/{name}
	if len(parts) >= 4 && parts[0] == "b" && parts[2] != "o" {
		ctx.ParentResource = parts[1] // bucket name
		ctx.ResourceType = parts[2]
		ctx.ResourceName = parts[3]
		return ctx, nil
	}

	// Object-scoped: b/{bucket}/o/{object}/{resourceType}/{name}
	// The object name may itself contain slashes, so it is read as everything
	// between the object marker and the trailing type and name rather than as a
	// single segment.
	if len(parts) >= 6 && parts[0] == "b" && parts[2] == "o" {
		object := strings.Join(parts[3:len(parts)-2], "/")
		ctx.ParentResource = fmt.Sprintf("%s/%s", parts[1], object) // bucket/object
		ctx.ResourceType = parts[len(parts)-2]
		ctx.ResourceName = parts[len(parts)-1]
		return ctx, nil
	}

	return ctx, fmt.Errorf("invalid storage native ID format: %s", nativeID)
}
