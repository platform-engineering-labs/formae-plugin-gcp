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
