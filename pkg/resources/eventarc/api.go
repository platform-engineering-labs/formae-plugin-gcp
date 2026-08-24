// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package eventarc

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// EventarcAPI - Eventarc API v1. Resources are location-scoped.
// create/delete are long-running operations (return an Operation to poll);
// get/list return the resource directly.
var EventarcAPI = base.APIConfig{
	BaseURL:     "https://eventarc.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: eventarcPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// EventarcOperations - asynchronous (LRO). create/delete return an Operation;
// formae polls Status() until the operation reports done.
var EventarcOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractEventarcNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// EventarcNativeID - full path
// "projects/{project}/locations/{location}/triggers/{name}".
var EventarcNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
}

// eventarcRequestTransformer drops "location", which addresses the resource in
// the URL and is not a body field. "name" stays: the engine reads the create id
// (?messageBusId=, ?pipelineId=) from the transformed body.
func eventarcRequestTransformer(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		if k == "location" {
			continue
		}
		body[k] = v
	}
	return body, nil
}

// pipelineRequestTransformer prepares a pipeline body. Beyond dropping the
// location it expands a destination's bare message-bus name into the full path
// the API wants. That is what lets a forma pass `bus.res.name` — a resolvable,
// so formae creates the bus first — instead of interpolating a path by hand,
// which would stringify the resolvable and lose the ordering edge.
func pipelineRequestTransformer(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	body, err := eventarcRequestTransformer(props, ctx)
	if err != nil {
		return nil, err
	}
	location, _ := props["location"].(string)
	if location == "" {
		location = ctx.Location
	}
	destinations, ok := body["destinations"].([]interface{})
	if !ok {
		return body, nil
	}
	expanded := make([]interface{}, 0, len(destinations))
	for _, raw := range destinations {
		dest, ok := raw.(map[string]interface{})
		if !ok {
			expanded = append(expanded, raw)
			continue
		}
		copied := make(map[string]interface{}, len(dest))
		for k, v := range dest {
			copied[k] = v
		}
		if bus, ok := copied["messageBus"].(string); ok && bus != "" && !strings.Contains(bus, "/") {
			copied["messageBus"] = fmt.Sprintf("projects/%s/locations/%s/messageBuses/%s",
				ctx.Project, location, bus)
		}
		expanded = append(expanded, copied)
	}
	body["destinations"] = expanded
	return body, nil
}

// pipelineResponseTransformer is the mirror of pipelineRequestTransformer: the
// request expands a bare bus name into a full path, so the read shortens it back
// again. Without the symmetry the declared value (a resolvable that resolves to
// the bus's short name) could never equal the value read back, and every
// comparison step would report drift on a resource that is in fact correct.
func pipelineResponseTransformer(props map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	out := locationResponseTransformer("pipelines")(props, ctx)
	destinations, ok := out["destinations"].([]interface{})
	if !ok {
		return out
	}
	shortened := make([]interface{}, 0, len(destinations))
	for _, raw := range destinations {
		dest, ok := raw.(map[string]interface{})
		if !ok {
			shortened = append(shortened, raw)
			continue
		}
		copied := make(map[string]interface{}, len(dest))
		for k, v := range dest {
			copied[k] = v
		}
		if bus, ok := copied["messageBus"].(string); ok {
			if i := strings.LastIndex(bus, "/messageBuses/"); i >= 0 {
				copied["messageBus"] = bus[i+len("/messageBuses/"):]
			}
		}
		shortened = append(shortened, copied)
	}
	out["destinations"] = shortened
	return out
}

// locationResponseTransformer shortens the resource name and puts back the
// location, which lives only in the returned path. Eventarc Advanced runs in a
// subset of regions, so these resources usually declare a location of their own
// rather than inheriting the target's - and a declared field the read never
// reports would look like it went missing.
//
// The collection is passed in so a path is only parsed when it is the expected
// kind: a trigger's path must not be read as a message bus.
func locationResponseTransformer(collection string) base.ResponseTransformerFunc {
	return func(props map[string]interface{}, _ base.TransformContext) map[string]interface{} {
		out := make(map[string]interface{}, len(props)+1)
		for k, v := range props {
			out[k] = v
		}
		if name, ok := props["name"].(string); ok {
			parts := strings.Split(name, "/")
			// projects/{p}/locations/{l}/{collection}/{name}
			if len(parts) == 6 && parts[2] == "locations" && parts[4] == collection {
				out["location"] = parts[3]
				out["name"] = parts[5]
			}
		}
		return out
	}
}

// eventarcPathBuilder builds
// /projects/{project}/locations/{location}/{resourceType}[/{name}].
// advancedCollections are the Eventarc Advanced collections that a forma pins to
// a specific region, because Advanced runs in a subset of regions and rarely the
// target's. Discovery would otherwise look only in the target's location and
// never find them; Eventarc accepts "locations/-" on list, so use it.
var advancedCollections = map[string]bool{"messageBuses": true, "pipelines": true}

func eventarcPathBuilder(ctx base.PathContext) string {
	if ctx.IsList && advancedCollections[ctx.ResourceType] {
		return fmt.Sprintf("/projects/%s/locations/-/%s", ctx.Project, ctx.ResourceType)
	}
	path := fmt.Sprintf("/projects/%s/locations/%s/%s", ctx.Project, ctx.Location, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
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

// extractEventarcNativeID builds the resource path. On async create the response
// is an Operation (not the resource), so build from context — where
// buildPathContext has already set ResourceName from the declared id. Fall back
// to the operation's metadata.target.
func extractEventarcNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if ctx.ResourceName != "" {
		return fmt.Sprintf("projects/%s/locations/%s/%s/%s",
			ctx.Project, ctx.Location, ctx.ResourceType, ctx.ResourceName)
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
