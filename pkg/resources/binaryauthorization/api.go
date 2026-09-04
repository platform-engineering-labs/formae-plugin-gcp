// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package binaryauthorization

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// gkePlatform is the only platform Binary Authorization v1 accepts in a
// platform policy path.
//
// The URL segment is a variable — .../platforms/{platform}/policies/{id} — but
// v1 has exactly one platform behind it. The discovery document exposes only
// projects.platforms.gke, an unknown platform id answers HTTP 400, and asking
// for "cloudRun" with a gkePolicy body is rejected outright:
//
//	The input is not valid: Platform mismatch between policy details and the
//	resource's platform id. Expected 'gke' platform_id, but found 'cloudRun'.
//
// (An error mentioning cloudRunPolicy does surface from the body validator, so
// a second platform is presumably coming; when it arrives this constant becomes
// a declared field and the native ID grows a component. Until then a field
// would only be a value with one legal setting that the API never echoes back
// — an input-only field, which reads as drift on every sync.)
const gkePlatform = "gke"

// BinaryAuthorizationAPI - Binary Authorization API v1.
//
// Everything in it is synchronous: attestors.create/update answer with the
// attestor, platforms.policies.create/replacePlatformPolicy answer with the
// policy, and both deletes answer HTTP 200 with an empty body. There is no
// Operation anywhere in the discovery document.
var BinaryAuthorizationAPI = base.APIConfig{
	BaseURL:     "https://binaryauthorization.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: binaryAuthorizationPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// BinaryAuthorizationOperations - synchronous, for the whole API.
//
// base.ResourceRegistry.Register only substitutes the registry-wide
// OperationConfig when a definition's OperationIDExtractor is nil, so
// extractOperationName is wired in even though this API never returns an
// Operation and it can therefore never match. Leaving it nil here would make
// every definition inherit whatever the registry was built with.
//
// The async path does not degrade for a create: extractOperationName would
// return "", Create would report InProgress with an empty RequestID, and the
// poll would GET the bare base URL forever. Hence Synchronous.
var BinaryAuthorizationOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractBinaryAuthorizationNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// BinaryAuthorizationNativeID - full path, in one of two shapes:
//
//	projects/{project}/attestors/{attestor}
//	projects/{project}/platforms/gke/policies/{policy}
var BinaryAuthorizationNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseBinaryAuthorizationNativeID,
}

// binaryAuthorizationPathBuilder builds /projects/{project}/attestors[/{name}]
// or /projects/{project}/platforms/gke/policies[/{name}].
//
// The platform segment is injected here rather than declared, because it is a
// constant: see gkePlatform.
func binaryAuthorizationPathBuilder(ctx base.PathContext) string {
	path := "/projects/" + ctx.Project
	if ctx.ResourceType == platformPolicyCollection {
		path += "/platforms/" + gkePlatform
	}
	path += "/" + ctx.ResourceType
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// parseBinaryAuthorizationNativeID restores the context a read needs.
//
// The default path parser walks key/value pairs, so "platforms/gke" would set
// ResourceType to "platforms" and then be overwritten by "policies", losing
// nothing — but "projects/{p}/platforms/gke" has an odd number of segments
// after the pairs, and relying on that accident is how a read ends up
// addressing the wrong collection. Parse the two shapes explicitly instead.
func parseBinaryAuthorizationNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 4 || parts[0] != "projects" {
		return base.PathContext{}, fmt.Errorf("invalid binary authorization native ID: %s", nativeID)
	}
	ctx := base.PathContext{Project: parts[1]}
	switch {
	case len(parts) == 4 && parts[2] == attestorCollection:
		ctx.ResourceType = parts[2]
		ctx.ResourceName = parts[3]
	case len(parts) == 6 && parts[2] == "platforms" && parts[4] == platformPolicyCollection:
		ctx.ResourceType = parts[4]
		ctx.ResourceName = parts[5]
	default:
		return base.PathContext{}, fmt.Errorf("invalid binary authorization native ID: %s", nativeID)
	}
	return ctx, nil
}

// extractBinaryAuthorizationNativeID builds the resource path.
//
// Every mutating call answers with the resource, whose "name" is already the
// full path, so that is the primary source; the context is the fallback for a
// create whose response has been shortened by a transformer before this runs.
func extractBinaryAuthorizationNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name, ok := response["name"].(string); ok && strings.HasPrefix(name, "projects/") {
		return name
	}
	if ctx.ResourceName == "" {
		return ""
	}
	if ctx.ResourceType == platformPolicyCollection {
		return fmt.Sprintf("projects/%s/platforms/%s/%s/%s",
			ctx.Project, gkePlatform, ctx.ResourceType, ctx.ResourceName)
	}
	return fmt.Sprintf("projects/%s/%s/%s", ctx.Project, ctx.ResourceType, ctx.ResourceName)
}

// extractOperationName is never satisfied by this API — nothing in it returns
// an Operation — and exists only so BinaryAuthorizationOperations is not
// discarded by the registry. See BinaryAuthorizationOperations.
func extractOperationName(response map[string]interface{}) string {
	if name, ok := response["name"].(string); ok && strings.Contains(name, "/operations/") {
		return name
	}
	return ""
}

// checkOperationStatus reports a polled Operation done, mapping a present
// "error" to a terminal failure. Unreachable for this API; kept so the config
// is complete rather than partly nil.
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
