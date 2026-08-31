// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package base

import (
	"strings"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// TransformContext provides context for transformers
type TransformContext struct {
	Project      string
	Region       string
	Zone         string
	Location     string
	ResourceType string
	Operation    resource.Operation

	// ParentResource is the resource this one hangs off, where there is one.
	// A nested resource's response often carries neither its parent nor its
	// project - both live only in the path - so a transformer needs them from
	// here to put back what a forma declared.
	ParentResource string
	ParentType     string
}

// RequestTransformer transforms request properties before sending to API
type RequestTransformer interface {
	Transform(props map[string]interface{}, ctx TransformContext) (map[string]interface{}, error)
}

// ResponseTransformer transforms API response properties before returning
type ResponseTransformer interface {
	Transform(apiResponse map[string]interface{}, ctx TransformContext) map[string]interface{}
}

// RequestTransformerFunc is a function adapter for RequestTransformer
type RequestTransformerFunc func(props map[string]interface{}, ctx TransformContext) (map[string]interface{}, error)

func (f RequestTransformerFunc) Transform(props map[string]interface{}, ctx TransformContext) (map[string]interface{}, error) {
	return f(props, ctx)
}

// ResponseTransformerFunc is a function adapter for ResponseTransformer
type ResponseTransformerFunc func(apiResponse map[string]interface{}, ctx TransformContext) map[string]interface{}

func (f ResponseTransformerFunc) Transform(apiResponse map[string]interface{}, ctx TransformContext) map[string]interface{} {
	return f(apiResponse, ctx)
}

// WrapperTransformer wraps request properties in a field
// Used by Container API which requires {"cluster": {...}} instead of {...}
type WrapperTransformer struct {
	WrapperField string
}

func (t *WrapperTransformer) Transform(props map[string]interface{}, ctx TransformContext) (map[string]interface{}, error) {
	if t.WrapperField == "" {
		return props, nil
	}
	return map[string]interface{}{
		t.WrapperField: props,
	}, nil
}

// FieldFilterTransformer filters fields from properties
type FieldFilterTransformer struct {
	ExcludeFields []string // Fields to exclude (blacklist)
	IncludeFields []string // Fields to include (whitelist, takes precedence)
}

func (t *FieldFilterTransformer) Transform(props map[string]interface{}, ctx TransformContext) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	if len(t.IncludeFields) > 0 {
		// Whitelist mode - only include specified fields
		for _, field := range t.IncludeFields {
			if val, ok := props[field]; ok {
				result[field] = val
			}
		}
	} else {
		// Blacklist mode - exclude specified fields
		excludeMap := make(map[string]bool)
		for _, field := range t.ExcludeFields {
			excludeMap[field] = true
		}

		for k, v := range props {
			if !excludeMap[k] {
				result[k] = v
			}
		}
	}

	return result, nil
}

// CompositeRequestTransformer chains multiple request transformers
type CompositeRequestTransformer struct {
	Transformers []RequestTransformer
}

func (t *CompositeRequestTransformer) Transform(props map[string]interface{}, ctx TransformContext) (map[string]interface{}, error) {
	result := props
	var err error
	for _, transformer := range t.Transformers {
		result, err = transformer.Transform(result, ctx)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// CompositeResponseTransformer chains multiple response transformers
type CompositeResponseTransformer struct {
	Transformers []ResponseTransformer
}

func (t *CompositeResponseTransformer) Transform(apiResponse map[string]interface{}, ctx TransformContext) map[string]interface{} {
	result := apiResponse
	for _, transformer := range t.Transformers {
		result = transformer.Transform(result, ctx)
	}
	return result
}

// PassThroughTransformer returns properties unchanged
type PassThroughTransformer struct{}

func (t *PassThroughTransformer) Transform(props map[string]interface{}, ctx TransformContext) (map[string]interface{}, error) {
	return props, nil
}

// ExtractLastSegment extracts the last segment from a URL path
// e.g., "https://www.googleapis.com/compute/v1/projects/my-project/regions/us-central1" -> "us-central1"
func ExtractLastSegment(url string) string {
	parts := strings.Split(url, "/")
	return parts[len(parts)-1]
}

// URLSegmentResponseTransformer extracts the last segment from URL fields in API responses
// This is useful for fields like "region", "zone", "machineType" that return full URLs
// e.g., "https://www.googleapis.com/compute/v1/projects/my-project/regions/us-central1" -> "us-central1"
type URLSegmentResponseTransformer struct {
	// Fields specifies which fields to transform (extract last segment from URL)
	// If empty, defaults to common GCP URL fields: region, zone, machineType
	Fields []string
}

// DefaultURLSegmentFields are the common GCP fields that contain full URLs
var DefaultURLSegmentFields = []string{"region", "zone", "machineType"}

func (t *URLSegmentResponseTransformer) Transform(apiResponse map[string]interface{}, ctx TransformContext) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy all fields
	for k, v := range apiResponse {
		result[k] = v
	}

	// Determine which fields to transform
	fields := t.Fields
	if len(fields) == 0 {
		fields = DefaultURLSegmentFields
	}

	// Extract last segment from URL fields
	for _, field := range fields {
		if val, ok := result[field].(string); ok && val != "" {
			result[field] = ExtractLastSegment(val)
		}
	}

	return result
}

// RegionResponseTransformer is a convenience transformer that extracts the region name from the full URL
// Use this for regional resources that return "region" as a full URL
var RegionResponseTransformer = &URLSegmentResponseTransformer{
	Fields: []string{"region"},
}

// ZoneResponseTransformer is a convenience transformer that extracts the zone name from the full URL
// Use this for zonal resources that return "zone" as a full URL
var ZoneResponseTransformer = &URLSegmentResponseTransformer{
	Fields: []string{"zone"},
}

// ProjectResponseTransformer adds the project from TransformContext to the API response
// This is useful for APIs that don't return the project in the response body
type ProjectResponseTransformer struct{}

func (t *ProjectResponseTransformer) Transform(apiResponse map[string]interface{}, ctx TransformContext) map[string]interface{} {
	apiResponse["project"] = ctx.Project
	return apiResponse
}

// AddProjectResponseTransformer is a convenience instance of ProjectResponseTransformer
var AddProjectResponseTransformer = &ProjectResponseTransformer{}

// DropFields returns a transformer that removes the named fields from every
// request body. Use it for properties that identify the resource's place in the
// URL rather than its payload — a nested resource's parent id and location —
// which several GCP APIs reject outright as unknown body fields. base builds
// the path context from the raw properties before any transformer runs, so
// removing them here does not affect routing.
func DropFields(fields ...string) RequestTransformer {
	drop := make(map[string]bool, len(fields))
	for _, f := range fields {
		drop[f] = true
	}
	return RequestTransformerFunc(
		func(props map[string]interface{}, _ TransformContext) (map[string]interface{}, error) {
			out := make(map[string]interface{}, len(props))
			for k, v := range props {
				if drop[k] {
					continue
				}
				out[k] = v
			}
			return out, nil
		})
}

// DropFieldsOnUpdate returns a transformer that removes the named fields from
// update bodies only. Several GCP APIs build their PATCH updateMask from the
// body's top-level fields, so an immutable or read-only field left in the body
// lands in the mask and the call is rejected outright (Cloud Logging answers
// "name cannot be changed"). Create bodies usually still need those fields —
// "name" typically carries the client-chosen id — hence the operation check.
func DropFieldsOnUpdate(fields ...string) RequestTransformer {
	drop := make(map[string]bool, len(fields))
	for _, f := range fields {
		drop[f] = true
	}
	return RequestTransformerFunc(
		func(props map[string]interface{}, ctx TransformContext) (map[string]interface{}, error) {
			if ctx.Operation != resource.OperationUpdate {
				return props, nil
			}
			out := make(map[string]interface{}, len(props))
			for k, v := range props {
				if drop[k] {
					continue
				}
				out[k] = v
			}
			return out, nil
		})
}

// ShortNameResponseTransformer rewrites a full-path "name" field
// (e.g. "projects/p/topics/my-topic") to its last segment ("my-topic"). Many
// GCP APIs (Pub/Sub, Secret Manager, DNS, IAM) echo the full resource path in
// "name", but users declare - and reconcile against - the short identifier.
// No-op when name is absent or already short.
var ShortNameResponseTransformer = ResponseTransformerFunc(func(apiResponse map[string]interface{}, _ TransformContext) map[string]interface{} {
	if name, ok := apiResponse["name"].(string); ok {
		if i := strings.LastIndex(name, "/"); i >= 0 {
			apiResponse["name"] = name[i+1:]
		}
	}
	return apiResponse
})
