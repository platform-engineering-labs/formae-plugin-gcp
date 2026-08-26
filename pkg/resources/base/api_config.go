// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package base

import "fmt"

// APIConfig defines the configuration for a GCP API
type APIConfig struct {
	// BaseURL is the base URL for the API (e.g., "https://compute.googleapis.com/compute/v1")
	BaseURL string

	// APIVersion for future version management
	APIVersion string

	// PathBuilder constructs the resource path based on context
	PathBuilder PathBuilderFunc

	// Pagination configures how pagination works for this API
	// If nil, defaults to Compute API style (maxResults/pageToken)
	Pagination *PaginationConfig
}

// PaginationConfig defines pagination parameter names for an API
type PaginationConfig struct {
	// Disabled completely disables pagination query parameters
	// Some APIs (like Container/GKE) don't support pagination parameters
	Disabled bool

	// PageSizeParam is the query parameter name for page size
	// Compute API uses "maxResults", Container/CloudRun use "pageSize"
	// Defaults to "maxResults" if empty
	PageSizeParam string

	// PageTokenParam is the query parameter name for page token
	// Most APIs use "pageToken"
	// Defaults to "pageToken" if empty
	PageTokenParam string
}

// IsPaginationDisabled returns true if pagination is disabled for this API
func (c *APIConfig) IsPaginationDisabled() bool {
	return c.Pagination != nil && c.Pagination.Disabled
}

// GetPageSizeParam returns the page size parameter name, defaulting to "maxResults"
func (c *APIConfig) GetPageSizeParam() string {
	if c.Pagination != nil && c.Pagination.PageSizeParam != "" {
		return c.Pagination.PageSizeParam
	}
	return "maxResults"
}

// GetPageTokenParam returns the page token parameter name, defaulting to "pageToken"
func (c *APIConfig) GetPageTokenParam() string {
	if c.Pagination != nil && c.Pagination.PageTokenParam != "" {
		return c.Pagination.PageTokenParam
	}
	return "pageToken"
}

// PathBuilderFunc constructs a resource path from context
type PathBuilderFunc func(ctx PathContext) string

// PathContext contains all information needed to build a URL path
type PathContext struct {
	// Project information
	Project string

	// Location information (different APIs use different terms)
	Region   string // Compute API
	Zone     string // Compute API
	Location string // Container API uses "location" instead of region/zone

	// Resource information
	ResourceType string // Plural form (e.g., "instances", "clusters")
	ResourceName string // Empty for collection URLs

	// Parent resource information (for nested resources)
	ParentResource string // e.g., cluster name for nodePools
	ParentType     string // e.g., "clusters"

	// Custom segments for hierarchical APIs (e.g., Storage)
	CustomSegments []string

	// IsList marks a collection URL built for List rather than for create or
	// read. Discovery lists with no properties, so a path builder cannot tell
	// a declared location or parent from a target default - this can. Builders
	// use it to substitute an API's own wildcard (Eventarc's "locations/-")
	// where one exists, which is the difference between a resource that can be
	// discovered and one that can only be managed.
	IsList bool
}

// URLBuilder builds URLs for GCP API resources
type URLBuilder struct {
	apiConfig APIConfig
	context   PathContext
}

// NewURLBuilder creates a new URL builder
func NewURLBuilder(apiConfig APIConfig, context PathContext) *URLBuilder {
	return &URLBuilder{
		apiConfig: apiConfig,
		context:   context,
	}
}

// CollectionURL returns the URL for a resource collection (for POST/LIST)
func (b *URLBuilder) CollectionURL() string {
	ctx := b.context
	ctx.ResourceName = "" // Ensure name is empty for collection URL
	path := b.apiConfig.PathBuilder(ctx)
	return fmt.Sprintf("%s%s", b.apiConfig.BaseURL, path)
}

// ResourceURL returns the URL for a specific resource (for GET/PATCH/DELETE)
// If name is empty, returns collection URL
func (b *URLBuilder) ResourceURL(name string) string {
	if name == "" {
		return b.CollectionURL()
	}

	ctx := b.context
	ctx.ResourceName = name
	path := b.apiConfig.PathBuilder(ctx)
	return fmt.Sprintf("%s%s", b.apiConfig.BaseURL, path)
}

// WithContext returns a new URLBuilder with updated context
func (b *URLBuilder) WithContext(ctx PathContext) *URLBuilder {
	return &URLBuilder{
		apiConfig: b.apiConfig,
		context:   ctx,
	}
}

// WithParent returns a new URLBuilder with parent resource set (for nested resources)
func (b *URLBuilder) WithParent(parentType, parentResource string) *URLBuilder {
	ctx := b.context
	ctx.ParentType = parentType
	ctx.ParentResource = parentResource
	return &URLBuilder{
		apiConfig: b.apiConfig,
		context:   ctx,
	}
}
