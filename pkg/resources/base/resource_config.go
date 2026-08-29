// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package base

// UpdateMethod specifies the HTTP method to use for update operations
type UpdateMethod string

const (
	// UpdateMethodPatch uses PATCH for partial updates (default)
	// Only modified fields need to be provided
	UpdateMethodPatch UpdateMethod = "PATCH"

	// UpdateMethodPut uses PUT for full replacement
	// All resource properties must be provided
	// Required for some GCP resources like Compute instances where PATCH
	// doesn't support updating certain fields (labels, metadata)
	UpdateMethodPut UpdateMethod = "PUT"
)

// ResourceConfig defines the configuration for a GCP resource type
type ResourceConfig struct {
	// ResourceType is the plural API name (e.g., "instances", "clusters", "networks")
	ResourceType string

	// Scope defines how the resource is scoped (mutually exclusive with ParentResource)
	Scope *ScopeConfig

	// ParentResource defines parent/child relationship (mutually exclusive with Scope)
	ParentResource *ParentResourceConfig

	// SupportsUpdate indicates if the resource supports update operations
	SupportsUpdate bool

	// UpdateMethod specifies the HTTP method to use for updates (PATCH or PUT)
	// Defaults to PATCH if not specified
	// Use PUT for resources that require full replacement (e.g., Compute instances)
	UpdateMethod UpdateMethod

	// UpdateQueryParams specifies additional query parameters to include in update requests
	// This is useful for API-specific parameters like Compute Engine's mostDisruptiveAllowedAction
	// Example: map[string]string{"mostDisruptiveAllowedAction": "REFRESH"}
	UpdateQueryParams map[string]string

	// UpdateMaskFromBody, when true, appends an "updateMask" query parameter
	// listing the top-level fields of the (transformed) request body, e.g.
	// "?updateMask=labels,description". Required by many GCP PATCH endpoints
	// (Secret Manager, DNS, ...) that use a query-param field mask. Computed
	// from the body so a full-reconcile PATCH masks exactly the fields it sends.
	UpdateMaskFromBody bool

	// OptimisticLocking defines optimistic locking configuration
	OptimisticLocking *OptimisticLockingConfig

	// RequestWrapper wraps the request body in a field (e.g., "cluster", "nodePool")
	// Used by Container API which requires {"cluster": {...}} instead of {...}
	RequestWrapper string

	// ListItemsKey overrides the JSON key holding the array of items in a List
	// response. Defaults to trying "items" then the resource type. Set when the
	// API uses a different key (e.g. IAM serviceAccounts.list -> "accounts").
	ListItemsKey string

	// CreateIDParam, when set, sends the resource id as a create-time query
	// parameter (e.g. "repositoryId", "instanceId") instead of in the request
	// body. The id is taken from the "name" property, which is then removed from
	// the body. Common for location-scoped GCP APIs (Artifact Registry, Redis,
	// Filestore, ...).
	CreateIDParam string
}

// ScopeConfig defines how a resource is scoped
type ScopeConfig struct {
	Type ScopeType
}

// ScopeType defines the scoping model for a resource
type ScopeType string

const (
	// ScopeGlobal - Compute API global resources (e.g., networks, firewalls)
	ScopeGlobal ScopeType = "global"

	// ScopeRegional - Compute API regional resources (e.g., addresses, routers)
	ScopeRegional ScopeType = "regional"

	// ScopeZonal - Compute API zonal resources (e.g., instances, disks)
	ScopeZonal ScopeType = "zonal"

	// ScopeProjectLevel - SQL API resources scoped to project only
	ScopeProjectLevel ScopeType = "project"

	// ScopeLocationBased - Container API resources scoped to project/location
	ScopeLocationBased ScopeType = "location"
)

// ParentResourceConfig defines parent/child resource relationships
// Used for nested resources like Container nodePools under clusters
type ParentResourceConfig struct {
	// ParentType is the parent resource type (e.g., "clusters")
	ParentType string

	// PropertyName is the name of the property in the resource that contains the parent identifier
	// e.g., "cluster" for nodePools (even though ParentType is "clusters")
	// If empty, defaults to ParentType
	PropertyName string

	// RequiresParent indicates if the parent must be specified
	RequiresParent bool

	// ParentPathSegments for hierarchical APIs like Storage
	// e.g., ["b"] for bucket, ["b", "o"] for object
	ParentPathSegments []string

	// SecondPropertyName names a second property that, with PropertyName,
	// identifies the parent. A Cloud Storage object ACL hangs off a bucket AND
	// an object, and neither alone addresses it. When set, PathContext's
	// ParentResource carries the two joined by "/" - "{bucket}/{object}" -
	// which is the form the Storage path builder and native ID already expect.
	//
	// Leave empty for the usual single-property parent.
	SecondPropertyName string
}

// OptimisticLockingConfig defines optimistic locking behavior
type OptimisticLockingConfig struct {
	// Enabled indicates if optimistic locking is used
	Enabled bool

	// FieldName is the field used for locking
	// - "fingerprint" for Compute (firewalls, subnetworks)
	// - "metageneration" for Storage
	// - "etag" for other APIs
	FieldName string

	// LocationInURL indicates if the locking field is passed as query param (true)
	// or in the request body (false)
	LocationInURL bool
}

// GetUpdateMethod returns the HTTP method to use for updates, defaulting to PATCH
func (c *ResourceConfig) GetUpdateMethod() string {
	if c.UpdateMethod == UpdateMethodPut {
		return "PUT"
	}
	return "PATCH"
}
