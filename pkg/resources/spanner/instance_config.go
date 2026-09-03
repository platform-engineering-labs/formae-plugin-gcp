// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package spanner

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// instanceConfigUpdateMask is fixed rather than computed from the body.
// spanner.instanceConfigs.patch documents display_name and labels as the only
// updatable fields, and a full-reconcile PATCH has to be able to *clear* labels
// as well as set them - which only happens if "labels" is in the mask even when
// the body carries none. Deriving the mask from the fields present would make
// "remove every label" a silent no-op.
const instanceConfigUpdateMask = "displayName,labels"

// instanceConfigOutputOnly are the fields the API owns. Three of them
// (configType, etag, state) are exposed in the schema with hasProviderDefault
// so a read keeps them; they are still dropped from every request body, because
// echoing an etag back is what makes an API refuse a write as out of date.
//
// The rest are dropped from the response as well (see
// instanceConfigStripFromResponse): they are not declarable, and an undeclared
// field in stored state is drift nobody can resolve.
var instanceConfigOutputOnly = map[string]bool{
	"configType":                    true,
	"etag":                          true,
	"state":                         true,
	"optionalReplicas":              true,
	"reconciling":                   true,
	"leaderOptions":                 true,
	"freeInstanceAvailability":      true,
	"quorumType":                    true,
	"storageLimitPerProcessingUnit": true,
}

// instanceConfigStripFromResponse are the output-only fields the schema does
// NOT declare, so they must not reach stored state.
//
// optionalReplicas is the reason this list exists: a GET reports every replica
// location GCP offers for the base configuration - a very large array, each
// entry carrying a location, a type, a display name and a labels map. It is
// never user-declarable, and keeping it would bloat the state of every
// configuration with a catalogue that belongs to Google. The remaining names
// did not appear in the probe's GET but are documented Output only, so they are
// stripped defensively: a future API version that starts reporting one must not
// turn into permanent drift.
var instanceConfigStripFromResponse = []string{
	"optionalReplicas",
	"reconciling",
	"leaderOptions",
	"freeInstanceAvailability",
	"quorumType",
	"storageLimitPerProcessingUnit",
}

// instanceConfigRequestTransformer builds the two envelopes
// spanner.instanceConfigs uses:
//
//	create: {"instanceConfigId": "custom-x",
//	         "instanceConfig": {"name": "<full path>", ...}}
//	patch:  {"updateMask": "displayName,labels",
//	         "instanceConfig": {"name": "<full path>", "displayName": ..., "labels": ...}}
//
// Neither is expressible with the declarative hooks in base.ResourceConfig, and
// the reason is the order in which pkg/resources/base applies them. In
// base_resource.go's Create the sequence is: RequestTransformer, then
// CreateIDParam (which lifts body["name"] into a query parameter), then
// RequestWrapper (which wraps the WHOLE body). performUpdate in
// base_resource_helpers.go is the same shape: RequestTransformer, then
// UpdateMaskFromBody (which appends ?updateMask= from the body's top-level
// fields), then RequestWrapper.
//
// Because RequestWrapper wraps everything the transformer produced, there is no
// way to leave a sibling beside the wrapped object - and both envelopes need
// one: instanceConfigId on create, updateMask on patch. CreateIDParam is wrong
// twice over (the id belongs in the body, not the query string, and it would
// delete the "name" the body must also carry as a full path), and
// UpdateMaskFromBody is wrong because Spanner reads the mask from the body.
// So the transformer emits the finished envelope itself and the config sets
// none of RequestWrapper, CreateIDParam or UpdateMaskFromBody. This mirrors
// instanceRequestTransformer, which hit the same wall for the same reason.
//
// The full path inside the body is not redundant with instanceConfigId: a
// create that omitted instanceConfig.name was rejected 400 "Invalid
// CreateInstanceConfig request." with a fieldViolation on
// instance_config.name.
func instanceConfigRequestTransformer(
	props map[string]interface{}, ctx base.TransformContext,
) (map[string]interface{}, error) {
	name, _ := props["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("spanner instance config requires a name")
	}
	fullName := qualifyInstanceConfigPath(name, ctx.Project)

	if ctx.Operation == resource.OperationUpdate {
		// Send exactly what the mask names, plus the name that addresses the
		// configuration. Anything else in the body is either rejected or
		// silently ignored, and both are worse than not sending it.
		cfg := map[string]interface{}{"name": fullName}
		if v, ok := props["displayName"]; ok {
			cfg["displayName"] = v
		}
		if v, ok := props["labels"]; ok {
			cfg["labels"] = v
		}
		return map[string]interface{}{
			"updateMask":     instanceConfigUpdateMask,
			"instanceConfig": cfg,
		}, nil
	}

	cfg := make(map[string]interface{}, len(props))
	for k, v := range props {
		if instanceConfigOutputOnly[k] {
			continue
		}
		if k == "baseConfig" {
			v = qualifyBaseConfig(v, ctx.Project)
		}
		cfg[k] = v
	}
	cfg["name"] = fullName

	return map[string]interface{}{
		"instanceConfigId": name,
		"instanceConfig":   cfg,
	}, nil
}

// instanceConfigResponseTransformer shortens the two full paths a configuration
// reports and drops the output-only fields the schema does not declare.
//
// "name" keeps its "custom-" prefix: that prefix is part of the id Spanner
// requires, not a namespace the plugin adds.
func instanceConfigResponseTransformer(
	props map[string]interface{}, _ base.TransformContext,
) map[string]interface{} {
	out := copyProps(props)
	if name, ok := props["name"].(string); ok {
		out["name"] = lastSegment(name)
	}
	if baseCfg, ok := props["baseConfig"].(string); ok {
		out["baseConfig"] = lastSegment(baseCfg)
	}
	for _, k := range instanceConfigStripFromResponse {
		delete(out, k)
	}
	if replicas, ok := out["replicas"].([]interface{}); ok {
		out["replicas"] = normalizeReplicas(replicas)
	}
	return out
}

// normalizeReplicas drops a nested `defaultLeaderLocation: false`.
//
// The API omits the flag wherever it is false and reports it only on the one
// replica that carries it, so "omitted" is the canonical shape for false. A
// nested field cannot be marked hasProviderDefault - schema hints reach only
// top-level fields - so if a future API version ever started echoing the false
// case, every configuration would read back with a field no forma declared.
// Removing it here makes the read shape match the documented convention either
// way, and costs nothing today.
func normalizeReplicas(replicas []interface{}) []interface{} {
	out := make([]interface{}, 0, len(replicas))
	for _, raw := range replicas {
		replica, ok := raw.(map[string]interface{})
		if !ok {
			out = append(out, raw)
			continue
		}
		copied := make(map[string]interface{}, len(replica))
		for k, v := range replica {
			if k == "defaultLeaderLocation" {
				if flag, isBool := v.(bool); isBool && !flag {
					continue
				}
			}
			copied[k] = v
		}
		out = append(out, copied)
	}
	return out
}

// qualifyInstanceConfigPath builds the full path of an instance configuration
// from its short id, passing an already-qualified value through untouched.
func qualifyInstanceConfigPath(name, project string) string {
	if name == "" || strings.Contains(name, "/") {
		return name
	}
	return fmt.Sprintf("projects/%s/instanceConfigs/%s", project, name)
}

// qualifyBaseConfig is the inverse of the shortening
// instanceConfigResponseTransformer does: a forma names the base configuration
// by its bare id ("eur6") and the API wants the full path.
//
// Both halves are required. baseConfig is immutable, so a request that expands
// without a response that shortens leaves the declared value and the stored
// state permanently disagreeing - and every re-apply then plans a replacement
// that the API refuses.
func qualifyBaseConfig(v interface{}, project string) interface{} {
	baseCfg, ok := v.(string)
	if !ok {
		return v
	}
	return qualifyInstanceConfigPath(baseCfg, project)
}
