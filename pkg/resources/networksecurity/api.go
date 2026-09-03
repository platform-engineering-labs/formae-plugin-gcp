// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package networksecurity

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// defaultLocation is where a Network Security resource lives when it has no
// region of its own. Address groups, security profiles and security profile
// groups are global; URL lists are regional.
//
// The builders below read ctx.Location and fall back to this. A globally-scoped
// resource is registered with base.ScopeGlobal, which clears ctx.Location, so a
// target's configured region cannot leak into a global URL — it simply arrives
// here as empty.
const defaultLocation = "global"

// globalResourceTypes are the collections that live under locations/global
// whatever region the target is configured for.
//
// urlLists is deliberately absent: it is the one regional type in this API.
// Asked for locations/global it does not 404, it answers 400 "Invalid location
// in resource URL path", so getting this wrong fails every call rather than
// quietly using the wrong scope.
var globalResourceTypes = map[string]bool{
	"addressGroups":         true,
	"securityProfiles":      true,
	"securityProfileGroups": true,
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

// NetworkSecurityAPI - Network Security API v1. create/update/delete are
// long-running operations (return an Operation to poll); get/list return the
// resource directly.
var NetworkSecurityAPI = base.APIConfig{
	BaseURL:     "https://networksecurity.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: networkSecurityPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// NetworkSecurityOperations - asynchronous (LRO). Every mutating call returns
// an Operation; formae polls Status() until it reports done.
//
// Polling is not optional here even though these operations settle in seconds:
// the POST returning an Operation says only that the request was queued, not
// that the resource exists. Treating an accepted operation as a finished create
// is how a later read comes back not-found.
var NetworkSecurityOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractNetworkSecurityNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// NetworkSecurityNativeID - full path
// "projects/{project}/locations/{location}/{collection}/{name}".
var NetworkSecurityNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
}

// networkSecurityPathBuilder builds
// /projects/{project}/locations/{location}/{resourceType}[/{name}].
func networkSecurityPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s/%s", ctx.Project, locationOf(ctx), ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
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

// extractNetworkSecurityNativeID builds the resource path. On async create the
// response is an Operation (not the resource), so build from context —
// buildPathContext has already set ResourceName from the declared id. Fall back
// to the operation's metadata.target, then to a direct resource response.
func extractNetworkSecurityNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if ctx.ResourceName != "" {
		return fmt.Sprintf("projects/%s/locations/%s/%s/%s",
			ctx.Project, locationOf(ctx), ctx.ResourceType, ctx.ResourceName)
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
