// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package dataform implements GCP Dataform resources.
package dataform

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// DataformAPI - Dataform v1.
//
// v1 and v1beta1 both exist and describe the same collections at the same
// revision; v1 is used because it is the GA surface and its repositories
// collection has no deprecated deleteLongRunning twin. Everything is
// location-scoped: /projects/{p}/locations/{loc}/repositories/...
var DataformAPI = base.APIConfig{
	BaseURL:     "https://dataform.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: dataformPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// DataformOperations - synchronous, for the whole API.
//
// Dataform is unusual: not one of the four collections registered here uses a
// long-running operation. create and patch answer HTTP 200 with the resource
// itself and delete answers 200 with an empty body - verified live against all
// four. The API does expose projects.locations.operations, but nothing in the
// CRUD surface ever produces one.
//
// The async path does not degrade into this, which is why Synchronous is set
// rather than left at its zero value. extractOperationName only matches a name
// containing "/operations/", and these responses report the resource path, so
// the operation id would be "". BaseResource.Delete special-cases an empty
// operation id and reports success, but Create does not: it would report
// OperationStatusInProgress with an empty RequestID, and the following Status
// poll would GET the bare base URL - not an operation, and no report on the
// resource that was in fact created.
//
// OperationIDExtractor has to be non-nil for a different reason:
// ResourceRegistry.Register substitutes the registry-wide config only when a
// definition's OperationIDExtractor is nil. It is wired in below so this struct
// survives being used as the registry-wide config, even though nothing calls it.
var DataformOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractDataformNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// DataformNativeID - the full resource path, in one of two shapes:
//
//	projects/{p}/locations/{loc}/repositories/{repo}
//	projects/{p}/locations/{loc}/repositories/{repo}/{collection}/{name}
var DataformNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseDataformNativeID,
}

// parentCollectionOf names the collection a nested type hangs off, for the one
// case where PathContext cannot say: a List with no parent.
//
// Discovery lists with no properties at all, so nothing sets ParentType, and a
// workspace's URL would otherwise be ".../locations/{loc}/workspaces" - a
// collection that does not exist. The three nested types would then never be
// listed and would never appear in inventory.
//
// Dataform accepts "-" as a wildcard repository (verified live: GET
// .../locations/{loc}/repositories/-/workspaces answers 200 with the workspaces
// of every repository in the location), so one call enumerates them all and no
// walking provisioner is needed - the same shape Certificate Manager uses for
// certificate map entries.
var parentCollectionOf = map[string]string{
	"workspaces":      "repositories",
	"releaseConfigs":  "repositories",
	"workflowConfigs": "repositories",
}

// dataformLocation returns the location segment for a request. Dataform has no
// global scope: every collection lives under a real region, so the target's
// location is used as-is.
func dataformLocation(ctx base.PathContext) string {
	if ctx.Location != "" {
		return ctx.Location
	}
	return ctx.Region
}

// dataformPathBuilder builds
// /projects/{p}/locations/{loc}[/repositories/{repo}]/{resourceType}[/{name}].
func dataformPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s", ctx.Project, dataformLocation(ctx))
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

// parseDataformNativeID restores the context a read needs, including the
// repository a nested type hangs off. The default path parser walks key/value
// pairs and overwrites ResourceType as it goes, so a workspace's id would
// arrive with its repository silently dropped and the read would address
// ".../locations/{loc}/workspaces/{ws}", which 404s.
func parseDataformNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "repositories" {
		return base.PathContext{}, fmt.Errorf("invalid dataform native ID: %s", nativeID)
	}
	ctx := base.PathContext{
		Project:      parts[1],
		Location:     parts[3],
		Region:       parts[3],
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
		return base.PathContext{}, fmt.Errorf("invalid dataform native ID: %s", nativeID)
	}
	return ctx, nil
}

// extractDataformNativeID builds the resource path.
//
// The response is consulted before the context, unlike the async APIs where a
// create answers with an Operation that carries no path. Every Dataform
// response - create, patch, get and each item of a list - reports "name" as the
// full path, and that is the only source that works for a wildcard list: a List
// through "repositories/-" has no ParentResource in its context, so building
// from the context would produce a path with the wildcard still in it.
func extractDataformNativeID(response map[string]interface{}, ctx base.PathContext) string {
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
		ctx.Project, dataformLocation(ctx), parent, ctx.ResourceType, ctx.ResourceName)
}

// extractOperationName returns an LRO operation name if one is ever reported.
// Nothing in Dataform's CRUD surface produces one; see DataformOperations for
// why the hook is wired in regardless.
func extractOperationName(response map[string]interface{}) string {
	if name, ok := response["name"].(string); ok && strings.Contains(name, "/operations/") {
		return name
	}
	return ""
}

// checkOperationStatus reports whether a polled Operation is done, mapping a
// present "error" to a terminal failure. Unreachable while Synchronous is set,
// and kept so the config is complete.
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
