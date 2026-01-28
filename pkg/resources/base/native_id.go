// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package base

import (
	"fmt"
	"strings"
)

// NativeIDConfig defines how native IDs are formatted and parsed
type NativeIDConfig struct {
	// Format specifies the native ID format
	Format NativeIDFormat

	// PathTemplate for building native IDs in path/URL formats
	// e.g., "projects/{project}/instances/{name}"
	// e.g., "https://container.googleapis.com/v1/projects/{project}/locations/{location}/clusters/{name}"
	PathTemplate string

	// Parser extracts PathContext from a native ID
	Parser func(nativeID string) (PathContext, error)
}

// NativeIDFormat defines different native ID formats used by GCP APIs
type NativeIDFormat string

const (
	// SimpleNameFormat - Just the resource name (Compute style)
	// Example: "my-network"
	SimpleNameFormat NativeIDFormat = "name"

	// FullPathFormat - Full path without base URL (SQL style)
	// Example: "projects/my-project/instances/my-instance"
	FullPathFormat NativeIDFormat = "path"

	// FullURLFormat - Complete URL including base (Container style)
	// Example: "https://container.googleapis.com/v1/projects/my-project/locations/us-central1/clusters/my-cluster"
	FullURLFormat NativeIDFormat = "url"

	// HierarchicalFormat - Hierarchical path (Storage style)
	// Example: "my-bucket/my-folder/" or "accessId123"
	HierarchicalFormat NativeIDFormat = "hierarchical"
)

// BuildNativeID constructs a native ID based on configuration
func BuildNativeID(config NativeIDConfig, name string, ctx PathContext) string {
	switch config.Format {
	case SimpleNameFormat:
		return name

	case FullPathFormat, FullURLFormat:
		return expandTemplate(config.PathTemplate, ctx, name)

	case HierarchicalFormat:
		// For hierarchical formats, custom logic is needed
		// This is handled by API-specific implementations
		return name

	default:
		return name
	}
}

// ParseNativeID extracts components from a native ID
func ParseNativeID(config NativeIDConfig, nativeID string) (PathContext, error) {
	if config.Parser != nil {
		return config.Parser(nativeID)
	}

	// Default parsing for simple formats
	switch config.Format {
	case SimpleNameFormat:
		return PathContext{ResourceName: nativeID}, nil

	case FullPathFormat:
		return parsePathNativeID(nativeID)

	case FullURLFormat:
		return parseURLNativeID(nativeID)

	default:
		return PathContext{}, fmt.Errorf("no parser configured for native ID format: %s", config.Format)
	}
}

// expandTemplate replaces placeholders in a template with actual values
func expandTemplate(template string, ctx PathContext, name string) string {
	result := template
	replacements := map[string]string{
		"{project}":      ctx.Project,
		"{region}":       ctx.Region,
		"{zone}":         ctx.Zone,
		"{location}":     ctx.Location,
		"{resourceType}": ctx.ResourceType,
		"{name}":         name,
		"{parent}":       ctx.ParentResource,
		"{parentType}":   ctx.ParentType,
	}

	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}

	return result
}

// parsePathNativeID parses a path-format native ID
// Example: "projects/my-project/instances/my-instance"
func parsePathNativeID(nativeID string) (PathContext, error) {
	parts := strings.Split(nativeID, "/")
	ctx := PathContext{}

	for i := 0; i < len(parts)-1; i += 2 {
		key := parts[i]
		value := parts[i+1]

		switch key {
		case "projects":
			ctx.Project = value
		case "regions":
			ctx.Region = value
		case "zones":
			ctx.Zone = value
		case "locations":
			ctx.Location = value
		case "global":
			// No value, just indicates global scope
			continue
		default:
			// Assume it's the resource type and name
			ctx.ResourceType = key
			ctx.ResourceName = value
		}
	}

	return ctx, nil
}

// parseURLNativeID parses a full URL native ID
// Example: "https://container.googleapis.com/v1/projects/my-project/locations/us-central1/clusters/my-cluster"
func parseURLNativeID(nativeID string) (PathContext, error) {
	// Remove protocol and domain
	if idx := strings.Index(nativeID, "//"); idx != -1 {
		nativeID = nativeID[idx+2:]
	}
	if idx := strings.Index(nativeID, "/"); idx != -1 {
		nativeID = nativeID[idx+1:]
	}
	// Remove API version if present
	if strings.HasPrefix(nativeID, "v1/") || strings.HasPrefix(nativeID, "v2/") {
		nativeID = nativeID[3:]
	}

	// Now it looks like a path format
	return parsePathNativeID(nativeID)
}
