// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package cloudbuild

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// CloudBuildAPI - Cloud Build API v1.
//
// Triggers are addressed project-scoped, with no location segment:
//
//	/projects/{project}/triggers[/{trigger}]
//
// The API offers a second, location-scoped spelling of the same collection
// (/projects/{p}/locations/{loc}/triggers), and gcloud uses it with
// locations/global. Both were exercised live against the same trigger and both
// answered the same resource, so the choice is not about reachability - it is
// about which triggers a plugin creates and can then find again.
//
// The project-scoped path is pinned because it is unambiguously the global
// collection. The location-scoped path would take the target's configured
// region and create *regional* triggers, which a project-scoped list would then
// not return - the resource would be created and immediately invisible. Pinning
// the global spelling also matches what the API reports back: a trigger created
// this way answers with resourceName "projects/{p}/locations/global/triggers/{uuid}".
//
// Pagination uses pageSize/pageToken, and the list response keys its array on
// "triggers" - which is the collection segment, so base finds it without a
// ListItemsKey override.
var CloudBuildAPI = base.APIConfig{
	BaseURL:     "https://cloudbuild.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: cloudBuildPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// CloudBuildOperations - the registry-wide default: asynchronous.
//
// This is deliberately the async config even though the only type this package
// currently ships is synchronous. Cloud Build's *builds* surface really is
// long-running (builds.create, builds.retry and workerPools.* all answer with
// an Operation), so a registry-wide "synchronous" default would silently be
// wrong for whatever is added next. The one synchronous collection carries its
// own override instead - see CloudBuildSyncOperations.
var CloudBuildOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractCloudBuildNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// CloudBuildSyncOperations - for the triggers collection, which does not use
// long-running operations.
//
// Verified live against projects/development-477117: POST .../triggers answers
// HTTP 200 with the finished BuildTrigger (id, createTime and resourceName
// already assigned), PATCH answers 200 with the updated BuildTrigger, and
// DELETE answers 200 with an empty body after which a GET 404s immediately. The
// discovery document agrees: projects.triggers.create/patch declare a response
// of BuildTrigger and delete declares Empty, where projects.builds.create
// declares Operation.
//
// base substitutes the registry-wide OperationConfig only when a definition's
// OperationIDExtractor is nil, so this override must set that field even though
// it is never called - otherwise the whole struct is discarded. Leaving the
// async config in place would not degrade gracefully: extractOperationName only
// matches a name containing "/operations/", and a trigger's "name" is its short
// id, so the operation id would be "". Create does not special-case that - it
// reports InProgress with an empty RequestID and formae then polls
// "https://cloudbuild.googleapis.com/v1/", which is not an operation URL - so a
// create that in fact succeeded would be reported as failed.
var CloudBuildSyncOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractCloudBuildNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// CloudBuildNativeID - "projects/{project}/triggers/{trigger}".
//
// The trigger is keyed on its user-assigned name, not on the server-assigned
// uuid, and that was established live rather than assumed: GET, PATCH and
// DELETE at .../triggers/formae-probe-trig1 all answered the same resource as
// .../triggers/9cf38638-b297-42d5-acc3-e0cf85214cb5. The discovery document's
// {triggerId} path parameter accepts either.
//
// Keying on the name matters beyond tidiness: a native id built from the uuid
// could not be reconstructed from a forma, and a leaked trigger could not be
// found by a name-prefix sweep.
//
// The default path parser handles this shape - it walks key/value pairs, so
// "projects/p/triggers/t" yields Project=p, ResourceType=triggers,
// ResourceName=t - hence no custom Parser.
var CloudBuildNativeID = base.NativeIDConfig{
	Format:       base.FullPathFormat,
	PathTemplate: "projects/{project}/triggers/{name}",
}

// cloudBuildPathBuilder builds "/projects/{project}/triggers[/{trigger}]".
//
// ctx.Location and ctx.Region are deliberately unused: see CloudBuildAPI for
// why the global, project-scoped spelling is the one this plugin uses.
func cloudBuildPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/%s", ctx.Project, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractOperationName returns the LRO operation name from a mutating
// response. Never fires for a trigger - its "name" is a short id, not an
// operation path - and is present so the sync override survives registration,
// and so a future long-running collection in this API works unchanged.
func extractOperationName(response map[string]interface{}) string {
	if name, ok := response["name"].(string); ok && strings.Contains(name, "/operations/") {
		return name
	}
	return ""
}

// extractCloudBuildNativeID builds "projects/{project}/triggers/{name}".
//
// It cannot use the response's own path fields. A BuildTrigger reports
// "resourceName": "projects/{p}/locations/global/triggers/{uuid}", which is
// keyed on the server-assigned uuid and carries a locations segment this plugin
// does not address; a native id taken from it would not survive a re-read.
// "name" is the short, user-assigned id, which is exactly what the path needs.
//
// Prefer the declared name from context (set by base before create) and fall
// back to the response's "name", which is what a List item carries.
func extractCloudBuildNativeID(response map[string]interface{}, ctx base.PathContext) string {
	name := ctx.ResourceName
	if name == "" {
		name, _ = response["name"].(string)
	}
	if name == "" {
		return ""
	}
	resourceType := ctx.ResourceType
	if resourceType == "" {
		resourceType = "triggers"
	}
	return fmt.Sprintf("projects/%s/%s/%s", ctx.Project, resourceType, name)
}

// checkOperationStatus reports whether a polled Operation is done. Unused by
// the triggers collection (Status short-circuits on Synchronous) and kept for
// the same reason as extractOperationName.
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
