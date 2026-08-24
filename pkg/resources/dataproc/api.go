// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dataproc

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// DataprocAPI configuration for the Cloud Dataproc API v1.
//
// AutoscalingPolicies are REGION-scoped: they live under
// projects/{project}/regions/{region}/autoscalingPolicies (note "regions/",
// not "locations/"). List supports pageSize/pageToken pagination.
var DataprocAPI = base.APIConfig{
	BaseURL:     "https://dataproc.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: dataprocPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// DataprocOperations - autoscalingPolicies.create/delete return the resource
// (and Empty) directly, so operations are synchronous (no LRO polling).
var DataprocOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractDataprocNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// DataprocNativeID - full resource path
// "projects/{project}/regions/{region}/autoscalingPolicies/{name}".
var DataprocNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseDataprocNativeID,
}

// dataprocPathBuilder builds
// /projects/{project}/regions/{region}/{resourceType}[/{name}].
//
// Dataproc autoscalingPolicies are region-scoped, so the path uses
// "regions/{region}" rather than the location-based "locations/{location}".
func dataprocPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/regions/%s/%s", ctx.Project, ctx.Region, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractDataprocNativeID builds the full region-scoped resource path. The
// create/read response echoes the full path in "name" (output only); when
// absent we reconstruct it from context and the short id.
func extractDataprocNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name, ok := response["name"].(string); ok && strings.HasPrefix(name, "projects/") {
		return name
	}
	name := ctx.ResourceName
	if name == "" {
		if id, ok := response["id"].(string); ok {
			name = id
		}
	}
	if name == "" {
		return ""
	}
	return fmt.Sprintf("projects/%s/regions/%s/%s/%s", ctx.Project, ctx.Region, ctx.ResourceType, name)
}

// parseDataprocNativeID parses
// "projects/{project}/regions/{region}/{resourceType}/{name}".
func parseDataprocNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 6 || parts[0] != "projects" || parts[2] != "regions" {
		return base.PathContext{}, fmt.Errorf("invalid Dataproc native ID: %s", nativeID)
	}
	return base.PathContext{
		Project:      parts[1],
		Region:       parts[3],
		ResourceType: parts[4],
		ResourceName: parts[5],
	}, nil
}

// dataprocIDRequestTransformer maps the user-declared short identifier
// (PKL field "name") onto the request body field "id" that
// autoscalingPolicies.create expects IN THE BODY (not as a query param), and
// drops "name" (which is output-only on the API side). See resources.go for
// the rationale.
var dataprocIDRequestTransformer = base.RequestTransformerFunc(
	func(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
		body := make(map[string]interface{}, len(props))
		for k, v := range props {
			body[k] = v
		}
		if name, ok := body["name"].(string); ok && name != "" {
			body["id"] = name
			delete(body, "name")
		}
		return body, nil
	})

// DataprocLocationAPI is the same API with a location-based path. Dataproc is
// split: autoscalingPolicies live under "regions/{region}" while sessionTemplates
// live under "locations/{location}", so the two cannot share a path builder.
var DataprocLocationAPI = base.APIConfig{
	BaseURL:     "https://dataproc.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: dataprocLocationPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// DataprocLocationNativeID - full path
// "projects/{project}/locations/{location}/{resourceType}/{name}".
var DataprocLocationNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseDataprocLocationNativeID,
}

// dataprocLocationPathBuilder builds
// /projects/{project}/locations/{location}/{resourceType}[/{name}].
func dataprocLocationPathBuilder(ctx base.PathContext) string {
	location := ctx.Location
	if location == "" {
		location = ctx.Region
	}
	path := fmt.Sprintf("/projects/%s/locations/%s/%s", ctx.Project, location, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

func parseDataprocLocationNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 6 || parts[0] != "projects" || parts[2] != "locations" {
		return base.PathContext{}, fmt.Errorf("invalid dataproc location native ID: %s", nativeID)
	}
	return base.PathContext{
		Project:      parts[1],
		Location:     parts[3],
		Region:       parts[3],
		ResourceType: parts[4],
		ResourceName: parts[5],
	}, nil
}

// sessionTemplateRequestTransformer expands the declared short name into the full
// resource path. Session templates have no "?sessionTemplateId=" parameter -
// the API binds the id from the body's "name", and rejects the query parameter
// outright - so the path has to be built here.
func sessionTemplateRequestTransformer(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		body[k] = v
	}
	location := ctx.Location
	if location == "" {
		location = ctx.Region
	}
	if name, ok := body["name"].(string); ok && name != "" && !strings.Contains(name, "/") {
		body["name"] = fmt.Sprintf("projects/%s/locations/%s/sessionTemplates/%s",
			ctx.Project, location, name)
	}
	return body, nil
}

// sessionTemplateResponseTransformer is its mirror: the read shortens the name
// back so the declared value and the value read back can match.
func sessionTemplateResponseTransformer(props map[string]interface{}, _ base.TransformContext) map[string]interface{} {
	out := make(map[string]interface{}, len(props))
	for k, v := range props {
		out[k] = v
	}
	if name, ok := props["name"].(string); ok {
		if i := strings.LastIndex(name, "/sessionTemplates/"); i >= 0 {
			out["name"] = name[i+len("/sessionTemplates/"):]
		}
	}
	return out
}

// workflowTemplateResponseTransformer drops the server-assigned full path. A
// workflow template reports both "name" (the full path) and "id" (the short id a
// forma declares); keeping the path under "name" would look like an extra
// property with no declared counterpart, and the id is what identifies it.
func workflowTemplateResponseTransformer(props map[string]interface{}, _ base.TransformContext) map[string]interface{} {
	out := make(map[string]interface{}, len(props))
	for k, v := range props {
		if k == "name" {
			continue
		}
		out[k] = v
	}
	return out
}
