// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package logging

import (
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const (
	LogMetricResourceType        = "GCP::Logging::LogMetric"
	ProjectSinkResourceType      = "GCP::Logging::ProjectSink"
	ProjectExclusionResourceType = "GCP::Logging::ProjectExclusion"
	LogViewResourceType          = "GCP::Logging::LogView"
	LogBucketResourceType        = "GCP::Logging::LogBucket"
	SavedQueryResourceType       = "GCP::Logging::SavedQuery"
	LogScopeResourceType         = "GCP::Logging::LogScope"
)

// Location-scoped Logging responses carry neither "location" nor (for views)
// "bucket" — both live only inside the full resource name. Lift back whichever
// segments are present so the stored state matches the declared forma.
var pathPartsFromName = base.ResponseTransformerFunc(
	func(resp map[string]interface{}, _ base.TransformContext) map[string]interface{} {
		name, ok := resp["name"].(string)
		if !ok {
			return resp
		}
		parts := strings.Split(name, "/")
		for i := 0; i+1 < len(parts); i++ {
			switch parts[i] {
			case "locations":
				resp["location"] = parts[i+1]
			case "buckets":
				resp["bucket"] = parts[i+1]
			}
		}
		return resp
	})

var loggingRegistry *base.ResourceRegistry

func init() {
	loggingRegistry = base.NewResourceRegistry(LoggingAPI, LoggingOperations, LoggingNativeID)

	// LogMetric fits the generic engine: create is a plain POST to
	// projects/{p}/metrics with the client-assigned metric id in the body's
	// "name" field (no id query param, no transformer). Read/Delete operate on
	// the full resource path; delete returns Empty. All operations are sync.
	// Update (PUT) is deferred (SupportsUpdate: false); conformance skips it.
	err := loggingRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: LogMetricResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "metrics",
				Scope:          &base.ScopeConfig{Type: base.ScopeProjectLevel},
				SupportsUpdate: false,
			},
			ResponseTransformer: base.ShortNameResponseTransformer,
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
		},
		// ProjectSink also fits the generic engine: create POSTs to
		// projects/{p}/sinks with the client-chosen id in "name", and the
		// response already carries the short id (the full path comes back
		// separately as "resourceName"), so no name transformer is needed.
		{
			ResourceType: ProjectSinkResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "sinks",
				Scope:              &base.ScopeConfig{Type: base.ScopeProjectLevel},
				SupportsUpdate:     true,
				UpdateMethod:       base.UpdateMethodPatch,
				UpdateMaskFromBody: true, // sinks.patch requires updateMask
			},
			RequestTransformer: base.DropFieldsOnUpdate("name", "writerIdentity"),
		},
		// ProjectExclusion mirrors ProjectSink: same path shape, same
		// client-chosen id in "name", same updateMask-from-body patch.
		{
			ResourceType: ProjectExclusionResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "exclusions",
				Scope:              &base.ScopeConfig{Type: base.ScopeProjectLevel},
				SupportsUpdate:     true,
				UpdateMethod:       base.UpdateMethodPatch,
				UpdateMaskFromBody: true, // exclusions.patch requires updateMask
			},
			RequestTransformer: base.DropFieldsOnUpdate("name"),
		},
		// LogView - a filtered window onto a log bucket, used to grant access to
		// a subset of a bucket's entries.
		{
			// A log bucket is where log entries are actually retained; a view is
			// a window onto one, and a sink routes entries into one. The
			// project's _Default and _Required buckets are created by GCP, so
			// discovery reports two per project it did not create.
			ResourceType:    LogBucketResourceType,
			APIConfig:       LoggingViewAPI,
			OperationConfig: LoggingViewOperations,
			NativeIDConfig:  LoggingBucketNativeID,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "buckets",
				CreateIDParam:      "bucketId",
				SupportsUpdate:     true,
				UpdateMethod:       base.UpdateMethodPatch,
				UpdateMaskFromBody: true,
			},
			// "location" is a path component, not a body field, and would
			// otherwise land in the updateMask.
			RequestTransformer: &base.CompositeRequestTransformer{
				Transformers: []base.RequestTransformer{
					base.DropFields("location"),
					base.DropFieldsOnUpdate("name"),
				},
			},
			ResponseTransformer: base.ResponseTransformerFunc(logBucketResponseTransformer),
		},
		{
			ResourceType:    LogViewResourceType,
			APIConfig:       LoggingViewAPI,
			OperationConfig: LoggingViewOperations,
			NativeIDConfig:  LoggingViewNativeID,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "views",
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "buckets",
					PropertyName:   "bucket",
					RequiresParent: true,
				},
				CreateIDParam:      "viewId",
				SupportsUpdate:     true,
				UpdateMethod:       base.UpdateMethodPatch,
				UpdateMaskFromBody: true, // views.patch requires updateMask
			},
			// "bucket" and "location" are path components, not body fields, and
			// would otherwise land in the updateMask.
			RequestTransformer: &base.CompositeRequestTransformer{
				Transformers: []base.RequestTransformer{
					base.DropFields("bucket", "location"),
					base.DropFieldsOnUpdate("name"),
				},
			},
			ResponseTransformer: &base.CompositeResponseTransformer{
				Transformers: []base.ResponseTransformer{
					pathPartsFromName,
					base.ShortNameResponseTransformer,
				},
			},
		},
		// SavedQuery - a stored Logs Explorer query. Location-scoped with no
		// parent resource.
		{
			ResourceType:    SavedQueryResourceType,
			APIConfig:       LoggingLocationAPI,
			OperationConfig: LoggingLocationOperations,
			NativeIDConfig:  LocationScopedNativeID("savedQueries"),
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "savedQueries",
				CreateIDParam:      "savedQueryId",
				SupportsUpdate:     true,
				UpdateMethod:       base.UpdateMethodPatch,
				UpdateMaskFromBody: true, // savedQueries.patch requires updateMask
			},
			// "location" is a path component, not a body field.
			RequestTransformer: &base.CompositeRequestTransformer{
				Transformers: []base.RequestTransformer{
					base.DropFields("location"),
					base.DropFieldsOnUpdate("name"),
				},
			},
			ResponseTransformer: &base.CompositeResponseTransformer{
				Transformers: []base.ResponseTransformer{
					pathPartsFromName,
					base.ShortNameResponseTransformer,
				},
			},
		},
		// LogScope - names a set of views/buckets to query together, so a
		// Logs Explorer session can span projects. Same location-scoped shape
		// as SavedQuery.
		{
			ResourceType:    LogScopeResourceType,
			APIConfig:       LoggingLocationAPI,
			OperationConfig: LoggingLocationOperations,
			NativeIDConfig:  LocationScopedNativeID("logScopes"),
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "logScopes",
				CreateIDParam: "logScopeId",
				// ponytail: update is off. logScopes.patch works against the
				// live API under every updateMask tried (description alone, or
				// description+resourceNames in either order), but through the
				// plugin the PATCH reports success while the stored description
				// stays at its old value. SavedQuery — same package, same
				// location-scoped shape, same mask-from-body path — updates
				// correctly, so the difference is not the API. Diagnosing it
				// needs the plugin's request payload, which is not observable
				// today. A change replaces instead.
				SupportsUpdate: false,
			},
			RequestTransformer: &base.CompositeRequestTransformer{
				Transformers: []base.RequestTransformer{
					base.DropFields("location"),
					base.DropFieldsOnUpdate("name"),
				},
			},
			ResponseTransformer: &base.CompositeResponseTransformer{
				Transformers: []base.ResponseTransformer{
					pathPartsFromName,
					base.ShortNameResponseTransformer,
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}

	registerLogBucketOverrides()

	// Log views need a List that walks the buckets; see log_view_list.go.
	registerLogViewListOverride()
}
