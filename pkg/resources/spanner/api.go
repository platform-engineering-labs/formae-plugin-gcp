// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package spanner

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// SpannerAPI - Cloud Spanner API v1. Instances are project-scoped
// (projects/{project}/instances). create/patch are long-running operations
// (return an Operation to poll); get/list return the resource directly and
// delete returns Empty (handled as a synchronous completion by base.Delete).
var SpannerAPI = base.APIConfig{
	BaseURL:     "https://spanner.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: spannerPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// SpannerOperations - asynchronous (LRO). create returns an Operation; formae
// polls Status() until the operation reports done. delete returns Empty, i.e.
// no operation name, which base.Delete treats as an immediate success.
var SpannerOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractSpannerNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// SpannerNativeID - full path "projects/{project}/instances/{name}".
var SpannerNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
}

// spannerPathBuilder builds /projects/{project}/instances[/{name}].
func spannerPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/%s", ctx.Project, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractOperationName returns the LRO operation name from a create response
// ("projects/{p}/instances/{i}/operations/{op}"). base.Status GETs
// BaseURL + "/" + this to poll. A delete returns Empty ({}), so there is no
// name and this returns "" — base.Delete then reports immediate success.
func extractOperationName(response map[string]interface{}) string {
	if name, ok := response["name"].(string); ok && strings.Contains(name, "/operations/") {
		return name
	}
	return ""
}

// extractSpannerNativeID builds the resource path. On async create the response
// is an Operation (not the resource), so build from context — where
// buildPathContext has already set ResourceName from the declared id. Fall back
// to the operation's metadata.target, then to a direct resource response.
func extractSpannerNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if ctx.ResourceName != "" {
		return fmt.Sprintf("projects/%s/%s/%s", ctx.Project, ctx.ResourceType, ctx.ResourceName)
	}
	if md, ok := response["metadata"].(map[string]interface{}); ok {
		if target, ok := md["target"].(string); ok {
			if i := strings.Index(target, "projects/"); i >= 0 {
				return target[i:]
			}
		}
	}
	// Direct resource response (get/list): "name" is the full path.
	if name, ok := response["name"].(string); ok && !strings.Contains(name, "/operations/") {
		if i := strings.Index(name, "projects/"); i >= 0 {
			return name[i:]
		}
	}
	return ""
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
