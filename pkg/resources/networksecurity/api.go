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
// Two collections are deliberately absent, and both were established against
// the live API rather than assumed:
//
//   - urlLists is regional. Asked for locations/global it does not 404, it
//     answers 400 "Invalid location in resource URL path".
//   - gatewaySecurityPolicies, and the rules nested under them, are regional
//     too: locations/global answers 400 "Malformed name".
//
// Getting either wrong fails every call rather than quietly using the wrong
// scope, which is the reason this map is explicit instead of inferred.
var globalResourceTypes = map[string]bool{
	"addressGroups":                true,
	"securityProfiles":             true,
	"securityProfileGroups":        true,
	"clientTlsPolicies":            true,
	"serverTlsPolicies":            true,
	"backendAuthenticationConfigs": true,
	"authorizationPolicies":        true,
	"dnsThreatDetectors":           true,
}

// parentCollectionOf names the collection a nested type hangs off, for the one
// case where PathContext cannot say: a List with no parent.
//
// Discovery lists with no properties at all, so nothing sets ParentType, and
// the URL would otherwise be ".../locations/{region}/rules" - a collection that
// does not exist. The rules are then never listed and the resource never
// appears in inventory.
//
// Network Security accepts "-" as a wildcard parent (verified live: GET
// .../locations/{region}/gatewaySecurityPolicies/-/rules answers 200 with the
// rules of every policy in the region), so one call enumerates them all and no
// walking provisioner is needed - the same shape Certificate Manager uses for
// certificate map entries.
var parentCollectionOf = map[string]string{
	"rules": "gatewaySecurityPolicies",
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

// NetworkSecuritySyncOperations - the same API, for the one collection in it
// that does not use long-running operations. dnsThreatDetectors answers a POST
// and a PATCH with the resource itself, not an Operation.
//
// base can express this per resource: base.ResourceRegistry.Register only
// substitutes the registry-wide OperationConfig when the definition's
// OperationIDExtractor is nil, so a definition that carries this config keeps
// it. That matters, because the async path does not degrade gracefully here.
// A create whose response has no "/operations/" segment leaves
// extractOperationName returning "", and unlike Delete - which treats an empty
// operation id as "already finished" - Create reports InProgress with an empty
// RequestID. formae then polls BaseURL + "/", which is not an operation URL,
// and the create fails on a resource that was in fact created. So the override
// is required, not a tidiness.
//
// With Synchronous set, Create returns the transformed response directly,
// Update reads the resource back, Delete reports success on the 200, and Status
// short-circuits. The read-back matters for its own reason: this API's PATCH
// response is stale - a label added by a patch is absent from what the patch
// returns and present in the very next GET - so the read is the truth.
//
// Only the two mutating responses were observed directly. Delete was not, and
// AIP-151 puts a non-LRO delete with a non-LRO create; if that turns out wrong
// the failure mode is a destroy reported complete a second or two early.
var NetworkSecuritySyncOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractNetworkSecurityNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// NetworkSecurityNativeID - full path, in one of two shapes:
//
//	projects/{project}/locations/{location}/{collection}/{name}
//	projects/{project}/locations/{location}/gatewaySecurityPolicies/{policy}/rules/{rule}
var NetworkSecurityNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseNetworkSecurityNativeID,
}

// networkSecurityPathBuilder builds
// /projects/{project}/locations/{location}[/{parentType}/{parent}]/{resourceType}[/{name}].
func networkSecurityPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s", ctx.Project, locationOf(ctx))
	switch {
	case ctx.ParentType != "" && ctx.ParentResource != "":
		path += fmt.Sprintf("/%s/%s", ctx.ParentType, ctx.ParentResource)
	case parentCollectionOf[ctx.ResourceType] != "":
		// A List with no parent: see parentCollectionOf.
		path += fmt.Sprintf("/%s/-", parentCollectionOf[ctx.ResourceType])
	}
	path += "/" + ctx.ResourceType
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// parseNetworkSecurityNativeID restores the context a read needs, including the
// parent of a nested rule. The default path parser walks key/value pairs and
// overwrites ResourceType as it goes, so a rule's id would arrive with its
// parent silently dropped and the read would address
// ".../locations/{region}/rules/{rule}", which 404s.
func parseNetworkSecurityNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" {
		return base.PathContext{}, fmt.Errorf("invalid network security native ID: %s", nativeID)
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
		return base.PathContext{}, fmt.Errorf("invalid network security native ID: %s", nativeID)
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

// extractNetworkSecurityNativeID builds the resource path. On async create the
// response is an Operation (not the resource), so build from context —
// buildPathContext has already set ResourceName from the declared id, and the
// parent from the declared parent property. Fall back to the operation's
// metadata.target, then to a direct resource response.
func extractNetworkSecurityNativeID(response map[string]interface{}, ctx base.PathContext) string {
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
	// Direct resource response (get, or a list item): "name" is the full path,
	// and for a nested rule it already carries the parent segments.
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
