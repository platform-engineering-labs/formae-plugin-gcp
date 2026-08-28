// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package filestore

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// FilestoreAPI - Cloud Filestore API v1. Instances are location-scoped;
// create/patch/delete are long-running operations (return an Operation to
// poll); get/list return the resource directly.
var FilestoreAPI = base.APIConfig{
	BaseURL:     "https://file.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: filestorePathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// FilestoreOperations - asynchronous (LRO). create/delete return an Operation;
// formae polls Status() until the operation reports done.
var FilestoreOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractFilestoreNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// FilestoreNativeID - full path, either
// "projects/{p}/locations/{l}/instances/{name}" or the nested
// "projects/{p}/locations/{zone}/instances/{instance}/snapshots/{name}".
var FilestoreNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseFilestoreNativeID,
}

// parseFilestoreNativeID handles the location-scoped form (6 segments:
// instances and backups) and the nested form (8 segments: a snapshot inside an
// instance).
func parseFilestoreNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" {
		return base.PathContext{}, fmt.Errorf("invalid Filestore native ID: %s", nativeID)
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
			"invalid Filestore native ID: %s (expected 6 or 8 path segments, got %d)", nativeID, len(parts))
	}
}

// filestorePathBuilder builds
//
//	/projects/{p}/locations/{l}/{resourceType}[/{name}]
//	/projects/{p}/locations/{zone}/instances/{instance}/snapshots[/{name}]
//
// Filestore has no wildcard in the instance position, so a parentless list of
// snapshots would ask for an empty segment. Discovery lists with no parent, so
// snapshots walk the instances instead - see instance_walking_list.go.
func filestorePathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s", ctx.Project, ctx.Location)
	if ctx.ParentType != "" && ctx.ParentResource != "" {
		path = fmt.Sprintf("%s/%s/%s", path, ctx.ParentType, ctx.ParentResource)
	}
	path += "/" + ctx.ResourceType
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// dropFilestorePathFields removes the fields that address the resource in the
// URL and are not body fields. "name" stays: base.Create reads the create id
// (?backupId=, ?snapshotId=) out of it.
func dropFilestorePathFields(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		switch k {
		case "location", "instance":
			continue
		}
		body[k] = v
	}
	return body, nil
}

// backupRequest expands the source instance into the full path the API wants.
// A forma passes a resolvable - so formae creates the instance first and the
// backup gets the ordering edge - which resolves to a bare instance name.
// Interpolating the path by hand in the forma would stringify the resolvable
// and lose that edge.
func backupRequest(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	body, err := dropFilestorePathFields(props, ctx)
	if err != nil {
		return nil, err
	}
	location, _ := props["location"].(string)
	if location == "" {
		location = ctx.Location
	}
	if name, ok := body["sourceInstance"].(string); ok && name != "" && !strings.Contains(name, "/") {
		body["sourceInstance"] = fmt.Sprintf("projects/%s/locations/%s/instances/%s",
			ctx.Project, location, name)
	}
	return body, nil
}

// backupResponse is the mirror of backupRequest.
func backupResponse(props map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	out := locationResponseTransformer("backups")(props, ctx)
	if path, ok := out["sourceInstance"].(string); ok {
		if i := strings.LastIndex(path, "/instances/"); i >= 0 {
			out["sourceInstance"] = path[i+len("/instances/"):]
		}
	}
	return out
}

// locationResponseTransformer puts back the short name and the location. The
// API reports "name" as a full path and never reports "location" as a field of
// its own, but a forma declares it - and a declared field the read never
// reports would look like it went missing on every comparison.
func locationResponseTransformer(collection string) base.ResponseTransformerFunc {
	return func(props map[string]interface{}, _ base.TransformContext) map[string]interface{} {
		out := make(map[string]interface{}, len(props)+1)
		for k, v := range props {
			out[k] = v
		}
		name, ok := props["name"].(string)
		if !ok {
			return out
		}
		parts := strings.Split(name, "/")
		if len(parts) == 6 && parts[2] == "locations" && parts[4] == collection {
			out["location"] = parts[3]
			out["name"] = parts[5]
		}
		return out
	}
}

// snapshotResponseTransformer is the nested equivalent: a snapshot's path also
// carries the instance it lives in.
func snapshotResponseTransformer(props map[string]interface{}, _ base.TransformContext) map[string]interface{} {
	out := make(map[string]interface{}, len(props)+2)
	for k, v := range props {
		out[k] = v
	}
	name, ok := props["name"].(string)
	if !ok {
		return out
	}
	parts := strings.Split(name, "/")
	// projects/{p}/locations/{zone}/instances/{instance}/snapshots/{name}
	if len(parts) == 8 && parts[4] == "instances" && parts[6] == "snapshots" {
		out["location"] = parts[3]
		out["instance"] = parts[5]
		out["name"] = parts[7]
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

// extractFilestoreNativeID builds the resource path. On async create the
// response is an Operation (not the resource), so build from context — where
// buildPathContext has already set ResourceName from the declared id. Fall back
// to the operation's metadata.target, then to a direct resource response.
func extractFilestoreNativeID(response map[string]interface{}, ctx base.PathContext) string {
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
