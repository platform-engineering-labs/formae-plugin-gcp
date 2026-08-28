// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package datastream

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// DatastreamAPI - Datastream API v1. Resources are location-scoped.
// create/delete are long-running operations (return an Operation to poll);
// get/list return the resource directly.
var DatastreamAPI = base.APIConfig{
	BaseURL:     "https://datastream.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: datastreamPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// DatastreamOperations - asynchronous (LRO). create/delete return an
// Operation; formae polls Status() until the operation reports done.
var DatastreamOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractDatastreamNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// DatastreamNativeID - full path, either
// "projects/{p}/locations/{l}/connectionProfiles/{name}" or the nested
// "projects/{p}/locations/{l}/privateConnections/{pc}/routes/{name}".
var DatastreamNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseDatastreamNativeID,
}

// parseDatastreamNativeID handles the location-scoped form (6 segments) and the
// nested form (8 segments: a route inside a private connection).
func parseDatastreamNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" {
		return base.PathContext{}, fmt.Errorf("invalid Datastream native ID: %s", nativeID)
	}
	switch len(parts) {
	case 6:
		return base.PathContext{
			Project:      parts[1],
			Location:     parts[3],
			ResourceType: parts[4],
			ResourceName: parts[5],
		}, nil
	case 8:
		return base.PathContext{
			Project:        parts[1],
			Location:       parts[3],
			ParentType:     parts[4],
			ParentResource: parts[5],
			ResourceType:   parts[6],
			ResourceName:   parts[7],
		}, nil
	default:
		return base.PathContext{}, fmt.Errorf(
			"invalid Datastream native ID: %s (expected 6 or 8 path segments, got %d)", nativeID, len(parts))
	}
}

// datastreamPathBuilder builds
//
//	/projects/{p}/locations/{l}/{resourceType}[/{name}]
//	/projects/{p}/locations/{l}/privateConnections/{pc}/routes[/{name}]
//
// Creating a stream appends ?force=true. Datastream validates a stream against
// its source when it is created, and a conformance source profile points at a
// hostname that does not answer - so without force the create fails on
// validation rather than on anything this plugin does. force is only correct on
// create: CollectionURL is also what List builds, and IsList tells them apart.
func datastreamPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s", ctx.Project, ctx.Location)
	switch {
	case ctx.ParentType != "" && ctx.ParentResource != "":
		path = fmt.Sprintf("%s/%s/%s", path, ctx.ParentType, ctx.ParentResource)
	case ctx.IsList && nestedInPrivateConnection[ctx.ResourceType]:
		// Discovery lists with no parent to name. Datastream accepts "-" in the
		// private-connection position (verified live), so ask across every one
		// rather than emitting a path with no parent at all - which is a 404,
		// and made routes undiscoverable.
		path += "/privateConnections/-"
	}
	path += "/" + ctx.ResourceType
	if ctx.ResourceName != "" {
		return path + "/" + ctx.ResourceName
	}
	if !ctx.IsList && forceOnCreate[ctx.ResourceType] {
		path += "?force=true"
	}
	return path
}

// forceOnCreate are the collections whose create must skip Datastream's
// connectivity validation.
var forceOnCreate = map[string]bool{"streams": true}

// nestedInPrivateConnection are the collections that only exist underneath a
// private connection.
var nestedInPrivateConnection = map[string]bool{"routes": true}

// extractOperationName returns the LRO operation name from a create/delete
// response ("projects/{p}/locations/{l}/operations/{op}"). base.Status GETs
// BaseURL + "/" + this to poll.
func extractOperationName(response map[string]interface{}) string {
	if name, ok := response["name"].(string); ok && strings.Contains(name, "/operations/") {
		return name
	}
	return ""
}

// extractDatastreamNativeID builds the resource path. On async create the
// response is an Operation (not the resource), so build from context — where
// buildPathContext has already set ResourceName from the declared id. Fall back
// to the operation's metadata.target.
func extractDatastreamNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if ctx.ResourceName != "" {
		prefix := fmt.Sprintf("projects/%s/locations/%s", ctx.Project, ctx.Location)
		if ctx.ParentType != "" && ctx.ParentResource != "" {
			prefix = fmt.Sprintf("%s/%s/%s", prefix, ctx.ParentType, ctx.ParentResource)
		}
		return fmt.Sprintf("%s/%s/%s", prefix, ctx.ResourceType, ctx.ResourceName)
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
	return base.CheckLROStatus(op)
}

// dropDatastreamPathFields removes the field that addresses a nested resource
// in the URL. "name" stays: base.Create reads the create id out of it.
func dropDatastreamPathFields(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		if k == "privateConnection" {
			continue
		}
		body[k] = v
	}
	return body, nil
}

// streamRequest expands the two connection profiles a stream joins into the
// full paths the API wants. A forma passes resolvables - so formae creates both
// profiles first - and each resolves to a bare profile name.
func streamRequest(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	body, err := dropDatastreamPathFields(props, ctx)
	if err != nil {
		return nil, err
	}
	expandProfile(body, "sourceConfig", "sourceConnectionProfile", ctx)
	expandProfile(body, "destinationConfig", "destinationConnectionProfile", ctx)
	return body, nil
}

// streamResponse is the mirror of streamRequest. Without the symmetry the
// declared profile name could never equal the full path read back, and every
// comparison step would report drift on a stream that is in fact correct.
func streamResponse(props map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	out := base.ShortNameResponseTransformer.Transform(props, ctx)
	shortenProfile(out, "sourceConfig", "sourceConnectionProfile")
	shortenProfile(out, "destinationConfig", "destinationConnectionProfile")
	return out
}

func expandProfile(body map[string]interface{}, configKey, field string, ctx base.TransformContext) {
	config, ok := body[configKey].(map[string]interface{})
	if !ok {
		return
	}
	copied := make(map[string]interface{}, len(config))
	for k, v := range config {
		copied[k] = v
	}
	if name, ok := copied[field].(string); ok && name != "" && !strings.Contains(name, "/") {
		copied[field] = fmt.Sprintf("projects/%s/locations/%s/connectionProfiles/%s",
			ctx.Project, ctx.Location, name)
	}
	body[configKey] = copied
}

func shortenProfile(out map[string]interface{}, configKey, field string) {
	config, ok := out[configKey].(map[string]interface{})
	if !ok {
		return
	}
	copied := make(map[string]interface{}, len(config))
	for k, v := range config {
		copied[k] = v
	}
	if path, ok := copied[field].(string); ok {
		if i := strings.LastIndex(path, "/connectionProfiles/"); i >= 0 {
			copied[field] = path[i+len("/connectionProfiles/"):]
		}
	}
	out[configKey] = copied
}

// routeResponse puts back the private connection, which lives only in the path.
func routeResponse(props map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	// Read the full path first: ShortNameResponseTransformer rewrites "name" in
	// place, and the parent only exists in the long form.
	fullName, _ := props["name"].(string)
	out := base.ShortNameResponseTransformer.Transform(props, ctx)
	if name := fullName; name != "" {
		parts := strings.Split(name, "/")
		// projects/{p}/locations/{l}/privateConnections/{pc}/routes/{name}
		if len(parts) == 8 && parts[4] == "privateConnections" && parts[6] == "routes" {
			out["privateConnection"] = parts[5]
		}
	}
	return out
}
