// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package parametermanager

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// globalLocation is the only location this plugin addresses, and the reason is
// worth reading before anyone tries to add a regional parameter.
//
// Parameter Manager is a "regional endpoint" API: a parameter that lives in a
// region is only reachable on that region's own host,
//
//	https://parametermanager.<region>.rep.googleapis.com/v1/...
//
// while locations/global is only reachable on the plain host,
//
//	https://parametermanager.googleapis.com/v1/...
//
// Crossing the two answers HTTP 403 PERMISSION_DENIED, "Read access to project
// '<project>' was denied". That message reads exactly like a missing IAM grant
// and is not one - all three of these were measured live against
// development-477117 with roles/parametermanager.admin held:
//
//	GET parametermanager.googleapis.com/v1/.../locations/europe-central2/parameters  -> 403
//	GET parametermanager.europe-central2.rep.googleapis.com/v1/.../locations/global/parameters -> 403
//	GET parametermanager.googleapis.com/v1/.../locations/global/parameters           -> 200 {}
//	GET parametermanager.europe-central2.rep.googleapis.com/v1/.../locations/europe-central2/parameters -> 200 {}
//
// So the 403 is a wrong-host error wearing a permissions error's clothes. Do
// not respond to it by granting a role.
//
// base.APIConfig.BaseURL is a single constant string and PathBuilder returns
// only the path appended to it, so nothing in base can vary the host per
// request. Rather than smuggle a host into the path, this API is registered
// global-only: the location segment is this constant and never the target's
// region, so a target configured for europe-central2 still addresses
// locations/global on the plain host instead of walking into the 403.
//
// Regional parameters are therefore NOT supported. Supporting them needs a
// per-request host in base - either an APIConfig.HostBuilder(PathContext)
// alongside BaseURL, or letting PathBuilder return an absolute URL that
// URLBuilder does not prefix. Neither exists today.
const globalLocation = "global"

// parentCollectionOf names the collection a nested type hangs off, for the one
// case where PathContext cannot say: a List with no parent.
//
// Discovery lists with no properties at all, so nothing sets ParentType, and a
// version's URL would otherwise be ".../locations/global/versions", a
// collection that does not exist. The versions would then never be listed and
// the resource would never appear in inventory.
//
// Parameter Manager accepts "-" as a wildcard parent (verified live: GET
// .../locations/global/parameters/-/versions answers 200 with the versions of
// every parameter in the location), so one call enumerates them all and no
// walking provisioner is needed - the same shape Certificate Manager and
// Network Security use.
var parentCollectionOf = map[string]string{
	"versions": "parameters",
}

// ParameterManagerAPI - Parameter Manager API v1.
//
// Pagination is pageSize/pageToken, per the API's own discovery document.
var ParameterManagerAPI = base.APIConfig{
	BaseURL:     "https://parametermanager.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: parameterManagerPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// ParameterManagerOperations - synchronous, for the whole API.
//
// There is no Operation anywhere in Parameter Manager v1: its discovery
// document declares no projects.locations.operations collection at all, and
// every mutating method answers with the resource itself - parameters.create
// and parameters.patch return a Parameter, versions.create and versions.patch
// return a ParameterVersion, and both deletes return Empty. All of that was
// confirmed live rather than read off the document.
//
// Registering it synchronous is required, not tidier. The async path does not
// degrade: extractOperationName finds no "/operations/" segment, so the
// operation id is "", and BaseResource.Create does not special-case that - it
// reports InProgress with an empty RequestID, and the following Status poll
// GETs the bare base URL, which reports on nothing. Delete alone would have
// degraded correctly, because it does treat an empty operation id as finished.
//
// With Synchronous set, Create returns the transformed create response, Update
// reads the resource back, Delete reports success on the 200 and Status
// short-circuits. The read-back on update matters here for a second reason:
// this API's mutating responses are stale. A PATCH that sets labels answers
// without them and the very next GET has them, and a create with labels does
// the same - the server applies them in a follow-up write (the returned
// updateTime moves on between the create response and the first GET). See the
// Parameter registration in resources.go for what that costs a fixture.
var ParameterManagerOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractParameterManagerNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// ParameterManagerNativeID - full path, in one of two shapes:
//
//	projects/{project}/locations/global/parameters/{parameter}
//	projects/{project}/locations/global/parameters/{parameter}/versions/{version}
var ParameterManagerNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseParameterManagerNativeID,
}

// parameterManagerPathBuilder builds
// /projects/{project}/locations/global[/parameters/{parameter}]/{resourceType}[/{name}].
//
// The location is always globalLocation and never ctx.Location or ctx.Region -
// see the constant for why addressing a region on this host is a 403.
func parameterManagerPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s", ctx.Project, globalLocation)
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

// parseParameterManagerNativeID restores the context a read needs, including
// the parent of a nested version. base's default path parser walks key/value
// pairs and overwrites ResourceType as it goes, so a version's id would arrive
// with its parent silently dropped and the read would address
// ".../locations/global/versions/{version}", which 404s.
func parseParameterManagerNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" {
		return base.PathContext{}, fmt.Errorf("invalid parameter manager native ID: %s", nativeID)
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
		return base.PathContext{}, fmt.Errorf("invalid parameter manager native ID: %s", nativeID)
	}
	return ctx, nil
}

// extractParameterManagerNativeID builds the resource path.
//
// Every mutating response in this API is the resource, so its "name" is already
// the full path and carries the parent segments of a nested version - that is
// the primary source. The context is the fallback, for a response that somehow
// arrives without a name; buildPathContext has already set ResourceName from
// the declared id and the parent from the declared parent property.
func extractParameterManagerNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name, ok := response["name"].(string); ok {
		if i := strings.Index(name, "projects/"); i >= 0 {
			return name[i:]
		}
	}
	if ctx.ResourceName == "" {
		return ""
	}
	parent := ""
	if ctx.ParentType != "" && ctx.ParentResource != "" {
		parent = fmt.Sprintf("%s/%s/", ctx.ParentType, ctx.ParentResource)
	}
	return fmt.Sprintf("projects/%s/locations/%s/%s%s/%s",
		ctx.Project, globalLocation, parent, ctx.ResourceType, ctx.ResourceName)
}

// extractOperationName exists only to keep ParameterManagerOperations from
// being discarded: base.ResourceRegistry.Register substitutes the
// registry-wide OperationConfig whenever a definition's OperationIDExtractor is
// nil, so the field has to be set even where it is never consulted. This API
// has no operations collection, so a response name is never an operation name
// and this always returns "".
func extractOperationName(response map[string]interface{}) string {
	if name, ok := response["name"].(string); ok && strings.Contains(name, "/operations/") {
		return name
	}
	return ""
}

// checkOperationStatus is unreachable while Synchronous is set - Status
// short-circuits before polling - and is kept so the config is complete if this
// API ever grows long-running operations.
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
