// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package alloydb

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// AlloyDBAPI - AlloyDB API v1. Resources are location-scoped.
// create/delete are long-running operations (return an Operation to poll);
// get/list return the resource directly.
var AlloyDBAPI = base.APIConfig{
	BaseURL:     "https://alloydb.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: alloyDBPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// AlloyDBOperations - asynchronous (LRO). create/delete return an Operation;
// formae polls Status() until the operation reports done.
var AlloyDBOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractAlloyDBNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// AlloyDBNativeID - full path
// "projects/{project}/locations/{location}/clusters/{name}".
var AlloyDBNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
}

// alloyDBPathBuilder builds
// /projects/{project}/locations/{location}/{resourceType}[/{name}], or
// /projects/{project}/locations/{location}/{parentType}/{parent}/{resourceType}[/{name}]
// for resources nested under another (instances under a cluster).
func alloyDBPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s", ctx.Project, ctx.Location)
	parent, parentType := ctx.ParentResource, ctx.ParentType
	// Discovery lists with no properties, so it can name no cluster to look in -
	// and with no parent named, List leaves ParentType empty too, so both halves
	// of the segment have to be supplied here. instances.list accepts the "-"
	// wildcard for the cluster, which reports every instance in the location;
	// users.list does not, so that one walks the clusters itself (user_list.go).
	if parent == "" && ctx.IsList && ctx.ResourceType == "instances" {
		parent, parentType = "-", "clusters"
	}
	if parentType != "" && parent != "" {
		path += fmt.Sprintf("/%s/%s", parentType, parent)
	}
	path += "/" + ctx.ResourceType
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// AlloyDBInstanceNativeID - instances are two levels deep, so the default
// pairwise parser would lose the owning cluster.
var AlloyDBInstanceNativeID = ClusterScopedNativeID("instances")

// AlloyDBUserNativeID - users sit at the same depth as instances.
var AlloyDBUserNativeID = ClusterScopedNativeID("users")

// ClusterScopedNativeID returns a NativeIDConfig for
// "projects/{p}/locations/{loc}/clusters/{cluster}/{resourceType}/{id}". Each
// leaf type gets its own check so a native ID belonging to a sibling type is
// rejected rather than silently mis-parsed.
func ClusterScopedNativeID(resourceType string) base.NativeIDConfig {
	return base.NativeIDConfig{
		Format: base.FullPathFormat,
		Parser: func(nativeID string) (base.PathContext, error) {
			parts := strings.Split(nativeID, "/")
			if len(parts) != 8 || parts[0] != "projects" || parts[2] != "locations" ||
				parts[4] != "clusters" || parts[6] != resourceType {
				return base.PathContext{}, fmt.Errorf(
					"invalid AlloyDB %s native ID: %s", resourceType, nativeID)
			}
			return base.PathContext{
				Project:        parts[1],
				Location:       parts[3],
				ParentType:     "clusters",
				ParentResource: parts[5],
				ResourceType:   parts[6],
				ResourceName:   parts[7],
			}, nil
		},
	}
}

// AlloyDBUserOperations - unlike clusters and instances, users.create returns
// the User itself rather than an Operation, so this path is synchronous.
var AlloyDBUserOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractUserNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// extractUserNativeID takes the full resource name the API returns, falling
// back to composing it from context (delete returns an empty body).
func extractUserNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name, ok := response["name"].(string); ok {
		if i := strings.Index(name, "projects/"); i >= 0 {
			return name[i:]
		}
	}
	if ctx.ResourceName == "" || ctx.ParentResource == "" {
		return ""
	}
	return fmt.Sprintf("projects/%s/locations/%s/clusters/%s/users/%s",
		ctx.Project, ctx.Location, ctx.ParentResource, ctx.ResourceName)
}

// AlloyDBInstanceOperations - as AlloyDBOperations, but the native ID keeps the
// owning cluster in the path.
var AlloyDBInstanceOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractInstanceNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// extractInstanceNativeID builds the nested instance path. On async create the
// response is an Operation, so compose from context; fall back to the
// operation's metadata.target, then to a direct resource response.
func extractInstanceNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if ctx.ResourceName != "" && ctx.ParentResource != "" {
		return fmt.Sprintf("projects/%s/locations/%s/clusters/%s/instances/%s",
			ctx.Project, ctx.Location, ctx.ParentResource, ctx.ResourceName)
	}
	if md, ok := response["metadata"].(map[string]interface{}); ok {
		if target, ok := md["target"].(string); ok {
			if i := strings.Index(target, "projects/"); i >= 0 {
				return target[i:]
			}
		}
	}
	if name, ok := response["name"].(string); ok && !strings.Contains(name, "/operations/") {
		if i := strings.Index(name, "projects/"); i >= 0 {
			return name[i:]
		}
	}
	return ""
}

// extractOperationName returns the LRO operation name from a create/delete
// response ("projects/{p}/locations/{l}/operations/{op}"). base.Status GETs
// BaseURL + "/" + this to poll.
func extractOperationName(response map[string]interface{}) string {
	if name, ok := response["name"].(string); ok && strings.Contains(name, "/operations/") {
		return name
	}
	return ""
}

// extractAlloyDBNativeID builds the resource path. On async create the response
// is an Operation (not the resource), so build from context — where
// buildPathContext has already set ResourceName from the declared id. Fall back
// to the operation's metadata.target.
func extractAlloyDBNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if ctx.ResourceName != "" {
		return fmt.Sprintf("projects/%s/locations/%s/%s/%s",
			ctx.Project, ctx.Location, ctx.ResourceType, ctx.ResourceName)
	}
	if md, ok := response["metadata"].(map[string]interface{}); ok {
		if target, ok := md["target"].(string); ok {
			if i := strings.Index(target, "projects/"); i >= 0 {
				return target[i:]
			}
		}
	}
	// Direct resource response (get): "name" is the full path.
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
