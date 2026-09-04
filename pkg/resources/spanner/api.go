// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package spanner implements GCP Cloud Spanner resources.
package spanner

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// SpannerAPI - Cloud Spanner Admin API v1. Instances and instance
// configurations are project-scoped, not location-scoped: an instance's region
// is its `config`, and a user-managed configuration's geography is its
// `replicas` - neither is a path segment. Databases nest under instances,
// backup schedules under databases.
var SpannerAPI = base.APIConfig{
	BaseURL:     "https://spanner.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: spannerPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// SpannerOperations - asynchronous. Instance, instance-configuration and
// database creates return an Operation to poll, and an instance
// configuration's patch does too. Their deletes answer with an empty body and
// no operation, which base.Delete already treats as done.
var SpannerOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractSpannerNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// SpannerSyncOperations - backup schedules are synchronous: create returns the
// schedule itself, with no operation to poll.
var SpannerSyncOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractSpannerNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// SpannerNativeID - the full resource path, in one of four shapes:
//
//	projects/{p}/instanceConfigs/{c}
//	projects/{p}/instances/{i}
//	projects/{p}/instances/{i}/databases/{d}
//	projects/{p}/instances/{i}/databases/{d}/backupSchedules/{b}
var SpannerNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseSpannerNativeID,
}

// spannerPathBuilder builds
// /projects/{p}[/instances/{i}][/databases/{d}]/{resourceType}[/{name}].
//
// A project-scoped collection - "instances" or "instanceConfigs" - names no
// parent and so comes out as /projects/{p}/{resourceType}[/{name}].
//
// The immediate parent comes from ParentResource and, for a backup schedule,
// the instance above it from CustomSegments[0].
func spannerPathBuilder(ctx base.PathContext) string {
	path := "/projects/" + ctx.Project
	if len(ctx.CustomSegments) > 0 && ctx.CustomSegments[0] != "" {
		path += "/instances/" + ctx.CustomSegments[0]
	}
	if ctx.ParentType != "" && ctx.ParentResource != "" {
		path += fmt.Sprintf("/%s/%s", ctx.ParentType, ctx.ParentResource)
	}
	path += "/" + ctx.ResourceType
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// parseSpannerNativeID turns a native ID back into the context the URL builder
// needs. A nested resource has to restore its parents, or a read would address
// the project-level collection and 404.
func parseSpannerNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 4 || parts[0] != "projects" {
		return base.PathContext{}, fmt.Errorf("invalid spanner native ID: %s", nativeID)
	}
	// An instance configuration is a sibling collection of instances, not a
	// child of one: projects/{p}/instanceConfigs/{c} and nothing deeper.
	if parts[2] == "instanceConfigs" {
		if len(parts) != 4 {
			return base.PathContext{}, fmt.Errorf("invalid spanner native ID: %s", nativeID)
		}
		return base.PathContext{
			Project:      parts[1],
			ResourceType: parts[2],
			ResourceName: parts[3],
		}, nil
	}
	if parts[2] != "instances" {
		return base.PathContext{}, fmt.Errorf("invalid spanner native ID: %s", nativeID)
	}
	ctx := base.PathContext{
		Project:      parts[1],
		ResourceType: parts[2],
		ResourceName: parts[3],
	}
	switch len(parts) {
	case 4: // instance
	case 6: // database: .../instances/{i}/databases/{d}
		if parts[4] != "databases" {
			return base.PathContext{}, fmt.Errorf("invalid spanner native ID: %s", nativeID)
		}
		ctx.ParentType = "instances"
		ctx.ParentResource = parts[3]
		ctx.ResourceType = parts[4]
		ctx.ResourceName = parts[5]
	case 8: // backup schedule: .../databases/{d}/backupSchedules/{b}
		if parts[4] != "databases" || parts[6] != "backupSchedules" {
			return base.PathContext{}, fmt.Errorf("invalid spanner native ID: %s", nativeID)
		}
		ctx.CustomSegments = []string{parts[3]}
		ctx.ParentType = "databases"
		ctx.ParentResource = parts[5]
		ctx.ResourceType = parts[6]
		ctx.ResourceName = parts[7]
	default:
		return base.PathContext{}, fmt.Errorf("invalid spanner native ID: %s", nativeID)
	}
	return ctx, nil
}

// extractOperationName returns the LRO name from a create response. Spanner
// hangs operations off the resource being created
// ("projects/{p}/instances/{i}/operations/{op}"), so the check is for the
// "/operations/" segment rather than a prefix.
func extractOperationName(response map[string]interface{}) string {
	if name, ok := response["name"].(string); ok && strings.Contains(name, "/operations/") {
		return name
	}
	return ""
}

// extractSpannerNativeID prefers the full path the API reports. On an async
// create the response is an Operation, whose name carries the resource path in
// front of "/operations/" - a create's own path context is the fallback.
func extractSpannerNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name, ok := response["name"].(string); ok && name != "" {
		if i := strings.Index(name, "/operations/"); i >= 0 {
			name = name[:i]
		}
		if strings.HasPrefix(name, "projects/") {
			return name
		}
	}
	if ctx.ResourceName == "" {
		return ""
	}
	parents := ""
	if len(ctx.CustomSegments) > 0 && ctx.CustomSegments[0] != "" {
		parents += "instances/" + ctx.CustomSegments[0] + "/"
	}
	if ctx.ParentType != "" && ctx.ParentResource != "" {
		parents += ctx.ParentType + "/" + ctx.ParentResource + "/"
	}
	return fmt.Sprintf("projects/%s/%s%s/%s", ctx.Project, parents, ctx.ResourceType, ctx.ResourceName)
}

// checkOperationStatus reports whether a polled Operation is done, mapping a
// present "error" to a terminal failure.
func checkOperationStatus(op map[string]interface{}) (bool, error) {
	done, _ := op["done"].(bool)
	if !done {
		return false, nil
	}
	if errObj, ok := op["error"].(map[string]interface{}); ok {
		msg, _ := errObj["message"].(string)
		if msg == "" {
			msg = "operation failed"
		}
		return true, fmt.Errorf("%s", msg)
	}
	return true, nil
}
