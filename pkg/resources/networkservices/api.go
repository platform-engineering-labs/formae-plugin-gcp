// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package networkservices

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// defaultLocation is where a Network Services resource lives when it has no
// region of its own, and it is also the fallback when a target names no region.
//
// The Cloud Service Mesh half of this API - meshes, the four route kinds,
// endpoint policies, service LB policies - is global. The extension and
// gateway half is regional.
const defaultLocation = "global"

// globalResourceTypes are the collections that live under locations/global
// whatever region the target is configured for.
//
// This map is explicit rather than inferred because this API gives no signal
// when the scope is wrong. Every collection here answers HTTP 200 at BOTH
// locations/global and locations/{region} - a regional GET does not 400 and
// does not 404, it returns an empty list. The two are separate namespaces, and
// only one of them holds anything: verified live, a mesh created at
// locations/global is absent from the locations/europe-central2 list and a GET
// for it there answers 404 NOT_FOUND.
//
// So guessing regional here would not fail loudly. It would create resources
// the plugin can then never find, and discovery would report an empty project.
// Hence the pin, and hence the comment.
//
// A collection absent from this map is treated as regional and takes the
// target's region, which is what the load-balancer extension collections
// (lbTrafficExtensions, lbRouteExtensions and friends) need. Adding a global
// collection means adding it here; adding a regional one means leaving it out.
var globalResourceTypes = map[string]bool{
	"meshes":            true,
	"serviceLbPolicies": true,
	"endpointPolicies":  true,
	"httpRoutes":        true,
	"grpcRoutes":        true,
	"tcpRoutes":         true,
	"tlsRoutes":         true,
	"serviceBindings":   true,
}

// locationOf returns the location segment for a request: "global" for the
// collections above, and the target's region for the rest.
func locationOf(ctx base.PathContext) string {
	if globalResourceTypes[ctx.ResourceType] {
		return defaultLocation
	}
	if ctx.Location != "" {
		return ctx.Location
	}
	return defaultLocation
}

// NetworkServicesAPI - Network Services API v1. create/update/delete are
// long-running operations (return an Operation to poll); get/list return the
// resource directly.
var NetworkServicesAPI = base.APIConfig{
	BaseURL:     "https://networkservices.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: networkServicesPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// NetworkServicesOperations - asynchronous (LRO). Every mutating call returns
// an Operation; formae polls Status() until it reports done.
//
// Polling is not optional, and here that is not a theoretical point. A create
// on this API can answer HTTP 200 with a perfectly well-formed Operation whose
// "done" is false, and settle two seconds later as a terminal failure with the
// resource never created - observed live on a grpcRoute whose hostname was
// already claimed by another route in the same mesh:
//
//	"done": true,
//	"error": { "code": 3, "message": "Config validation failed", ... }
//
// The subsequent GET answered 404. Treating the accepted Operation as a
// finished create would have stored a resource that does not exist.
var NetworkServicesOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractNetworkServicesNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// NetworkServicesNativeID - full path:
//
//	projects/{project}/locations/{location}/{collection}/{name}
//
// and, for a nested collection, with its parent's segments in between:
//
//	projects/{project}/locations/{location}/{parent}/{p}/{collection}/{name}
var NetworkServicesNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseNetworkServicesNativeID,
}

// networkServicesPathBuilder builds
// /projects/{project}/locations/{location}[/{parentType}/{parent}]/{resourceType}[/{name}].
//
// The parent segments are carried for the nested collections in this API
// (wasmPlugins/{plugin}/versions being the shape); a type with no parent
// configured never sets them and the path is the flat form.
func networkServicesPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s", ctx.Project, locationOf(ctx))
	if ctx.ParentType != "" && ctx.ParentResource != "" {
		path += fmt.Sprintf("/%s/%s", ctx.ParentType, ctx.ParentResource)
	}
	path += "/" + ctx.ResourceType
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// parseNetworkServicesNativeID restores the context a read needs, including the
// parent of a nested resource. The default path parser walks key/value pairs
// and overwrites ResourceType as it goes, so a nested resource's id would
// arrive with its parent silently dropped and the read would address a
// collection that does not exist.
func parseNetworkServicesNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" {
		return base.PathContext{}, fmt.Errorf("invalid network services native ID: %s", nativeID)
	}
	ctx := base.PathContext{
		Project:      parts[1],
		Location:     parts[3],
		ResourceType: parts[4],
		ResourceName: parts[5],
	}
	switch len(parts) {
	case 6:
	case 8:
		ctx.ParentType = parts[4]
		ctx.ParentResource = parts[5]
		ctx.ResourceType = parts[6]
		ctx.ResourceName = parts[7]
	default:
		return base.PathContext{}, fmt.Errorf("invalid network services native ID: %s", nativeID)
	}
	return ctx, nil
}

// extractOperationName returns the LRO operation name from a mutating response
// ("projects/{p}/locations/{loc}/operations/{op}"). base.Status GETs
// BaseURL + "/" + this to poll.
func extractOperationName(response map[string]interface{}) string {
	if name, ok := response["name"].(string); ok && strings.Contains(name, "/operations/") {
		return name
	}
	return ""
}

// extractNetworkServicesNativeID builds the resource path. On async create the
// response is an Operation, not the resource, so build from context -
// buildPathContext has already set ResourceName from the declared id. Fall back
// to the operation's metadata.target, then to a direct resource response.
func extractNetworkServicesNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if ctx.ResourceName != "" {
		parent := ""
		if ctx.ParentType != "" && ctx.ParentResource != "" {
			parent = fmt.Sprintf("%s/%s/", ctx.ParentType, ctx.ParentResource)
		}
		return fmt.Sprintf("projects/%s/locations/%s/%s%s/%s",
			ctx.Project, locationOf(ctx), parent, ctx.ResourceType, ctx.ResourceName)
	}
	if md, ok := response["metadata"].(map[string]interface{}); ok {
		if target, ok := md["target"].(string); ok {
			if i := strings.Index(target, "projects/"); i >= 0 {
				return target[i:]
			}
		}
	}
	// Direct resource response (get, or a list item): "name" is the full path.
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
