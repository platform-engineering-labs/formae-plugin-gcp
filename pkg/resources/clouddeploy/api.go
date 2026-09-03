// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package clouddeploy implements GCP Cloud Deploy resources.
package clouddeploy

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// parentCollectionOf names the collection a nested type hangs off, for the one
// case where PathContext cannot say: a List with no parent.
//
// Discovery lists an automation with no properties at all, so nothing sets
// ParentType, and the URL would otherwise be ".../locations/{region}/automations"
// - a collection that does not exist. The automations are then never listed and
// the resource never appears in inventory.
//
// Cloud Deploy accepts "-" as a wildcard parent, verified live: GET
// .../locations/europe-central2/deliveryPipelines/-/automations answers 200 with
// the automations of every pipeline in the region, each carrying its own
// pipeline in the returned name. One call enumerates them all, so no walking
// provisioner is needed - the same shape Certificate Manager uses for
// certificate map entries.
var parentCollectionOf = map[string]string{
	"automations": "deliveryPipelines",
}

// locationOf returns the location segment for a request.
//
// Cloud Deploy is regional throughout - there is no locations/global in this
// API - so the target's location is used, falling back to its region. The
// fallback exists because a target may configure only `region`; a Cloud Deploy
// URL with an empty location segment addresses nothing.
func locationOf(ctx base.PathContext) string {
	if ctx.Location != "" {
		return ctx.Location
	}
	return ctx.Region
}

// CloudDeployAPI - Cloud Deploy API v1. create/patch/delete are long-running
// operations (return an Operation to poll); get/list return the resource
// directly.
var CloudDeployAPI = base.APIConfig{
	BaseURL:     "https://clouddeploy.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: cloudDeployPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// CloudDeployOperations - asynchronous (LRO). Every mutating call in this API
// returns an Operation; formae polls Status() until it reports done.
//
// Polling is not optional here, and this API makes the reason unusually plain.
// Two things were observed live:
//
//   - A create that was accepted with HTTP 200 finished as a *failed*
//     operation: the operation came back done with an "error" carrying
//     "is not a valid resource ID for resource type stage.targetId", and the
//     pipeline it named never existed (a subsequent GET answered 404). The
//     accepted operation says only that the request was queued.
//   - Cloud Deploy serialises operations per resource. A second mutating call
//     issued while the first is still in flight is refused outright with
//     HTTP 409 ABORTED "unable to queue the operation". Waiting for done is
//     therefore also what makes back-to-back patches work at all.
var CloudDeployOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractCloudDeployNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// CloudDeployNativeID - full path, in one of two shapes:
//
//	projects/{project}/locations/{location}/{collection}/{name}
//	projects/{project}/locations/{location}/deliveryPipelines/{pipeline}/automations/{automation}
var CloudDeployNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseCloudDeployNativeID,
}

// cloudDeployPathBuilder builds
// /projects/{project}/locations/{location}[/{parentType}/{parent}]/{resourceType}[/{name}].
func cloudDeployPathBuilder(ctx base.PathContext) string {
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

// parseCloudDeployNativeID restores the context a read needs, including the
// parent of a nested automation. The default path parser walks key/value pairs
// and overwrites ResourceType as it goes, so an automation's id would arrive
// with its pipeline silently dropped and the read would address
// ".../locations/{region}/automations/{automation}", which 404s.
func parseCloudDeployNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" {
		return base.PathContext{}, fmt.Errorf("invalid cloud deploy native ID: %s", nativeID)
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
		return base.PathContext{}, fmt.Errorf("invalid cloud deploy native ID: %s", nativeID)
	}
	return ctx, nil
}

// extractOperationName returns the LRO operation name from a mutating response
// ("projects/{p}/locations/{loc}/operations/{op}").
func extractOperationName(response map[string]interface{}) string {
	if name, ok := response["name"].(string); ok && strings.Contains(name, "/operations/") {
		return name
	}
	return ""
}

// extractCloudDeployNativeID builds the resource path. On async create the
// response is an Operation rather than the resource, so build from context -
// buildPathContext has already set ResourceName from the declared id, and the
// parent from the declared parent property. Fall back to the operation's
// metadata.target, which Cloud Deploy fills with the resource being acted on,
// then to a direct resource response.
func extractCloudDeployNativeID(response map[string]interface{}, ctx base.PathContext) string {
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
	// and for a nested automation it already carries the pipeline segments.
	if name, ok := response["name"].(string); ok && !strings.Contains(name, "/operations/") {
		if i := strings.Index(name, "projects/"); i >= 0 {
			return name[i:]
		}
	}
	return ""
}

// checkOperationStatus reports whether a polled Operation is done, mapping a
// present "error" to a terminal failure. Cloud Deploy really does answer a
// finished operation with an error body - see CloudDeployOperations - so this
// is the check that turns a rejected create into a failed create rather than a
// silent success.
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
