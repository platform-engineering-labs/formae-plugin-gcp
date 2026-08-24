// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package artifactregistry

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// ArtifactRegistryAPI - Artifact Registry API v1. Resources are location-scoped.
// create/delete are long-running operations (return an Operation to poll);
// get/list return the resource directly.
var ArtifactRegistryAPI = base.APIConfig{
	BaseURL:     "https://artifactregistry.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: artifactRegistryPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// ArtifactRegistryOperations - asynchronous (LRO). create/delete return an
// Operation; formae polls Status() until the operation reports done.
var ArtifactRegistryOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractArtifactRegistryNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// ArtifactRegistryNativeID - full path
// "projects/{project}/locations/{location}/repositories/{name}".
var ArtifactRegistryNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseArtifactRegistryNativeID,
}

// parseArtifactRegistryNativeID turns a native ID back into the context the URL
// builder needs. Both shapes appear:
//
//	projects/{p}/locations/{l}/repositories/{repo}
//	projects/{p}/locations/{l}/repositories/{repo}/rules/{rule}
//
// The nested one has to restore ParentType/ParentResource, or a read would
// address the location-level collection and 404.
func parseArtifactRegistryNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" {
		return base.PathContext{}, fmt.Errorf("invalid artifact registry native ID: %s", nativeID)
	}
	ctx := base.PathContext{
		Project:      parts[1],
		Location:     parts[3],
		ResourceType: parts[4],
		ResourceName: parts[5],
	}
	if len(parts) == 8 {
		ctx.ParentType = parts[4]
		ctx.ParentResource = parts[5]
		ctx.ResourceType = parts[6]
		ctx.ResourceName = parts[7]
	} else if len(parts) != 6 {
		return base.PathContext{}, fmt.Errorf("invalid artifact registry native ID: %s", nativeID)
	}
	return ctx, nil
}

// artifactRegistryPathBuilder builds
// /projects/{project}/locations/{location}/{resourceType}[/{name}], inserting
// the parent when the resource is nested - a rule lives under its repository:
// /projects/{p}/locations/{l}/repositories/{repo}/rules/{rule}.
func artifactRegistryPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s", ctx.Project, ctx.Location)
	if ctx.ParentType != "" && ctx.ParentResource != "" {
		path += fmt.Sprintf("/%s/%s", ctx.ParentType, ctx.ParentResource)
	}
	path += "/" + ctx.ResourceType
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// ArtifactRegistryRuleOperations - rules are synchronous: create returns the
// rule itself and delete returns an empty body, with no operation to poll.
var ArtifactRegistryRuleOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractArtifactRegistryNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// ruleRequestTransformer drops the properties that address the rule rather than
// describe it. "repository" and "location" are path components, and the API
// rejects either as an unknown body field. "name" stays: the engine reads the
// create id (?ruleId=) from the transformed body, so dropping it here would send
// an empty id.
func ruleRequestTransformer(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		switch k {
		case "repository", "location":
			continue
		}
		body[k] = v
	}
	return body, nil
}

// ruleResponseTransformer puts back what the API leaves in the resource path.
// A rule reports only its full name plus action/operation/condition, so the
// repository and location a forma declares would otherwise look absent.
func ruleResponseTransformer(props map[string]interface{}, _ base.TransformContext) map[string]interface{} {
	out := make(map[string]interface{}, len(props)+2)
	for k, v := range props {
		out[k] = v
	}
	if name, ok := props["name"].(string); ok {
		parts := strings.Split(name, "/")
		// projects/{p}/locations/{l}/repositories/{repo}/rules/{rule}
		if len(parts) == 8 && parts[2] == "locations" && parts[4] == "repositories" && parts[6] == "rules" {
			out["location"] = parts[3]
			out["repository"] = parts[5]
			out["name"] = parts[7]
		}
	}
	return out
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

// extractArtifactRegistryNativeID builds the resource path. On async create the
// response is an Operation (not the resource), so build from context — where
// buildPathContext has already set ResourceName from the declared id. Fall back
// to the operation's metadata.target.
func extractArtifactRegistryNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if ctx.ResourceName != "" {
		// A nested resource keeps its parent in the id, or a later read would
		// address the location-level collection and find nothing.
		parent := ""
		if ctx.ParentType != "" && ctx.ParentResource != "" {
			parent = fmt.Sprintf("%s/%s/", ctx.ParentType, ctx.ParentResource)
		}
		return fmt.Sprintf("projects/%s/locations/%s/%s%s/%s",
			ctx.Project, ctx.Location, parent, ctx.ResourceType, ctx.ResourceName)
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
