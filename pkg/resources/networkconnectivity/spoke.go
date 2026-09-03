// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package networkconnectivity

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// spokeDropOnUpdate removes from a PATCH body everything the API refuses to
// have in an update mask. In practice a spoke accepts a change to `description`
// and `labels` and nothing else: `hub` and `linkedVpcNetwork` are fixed at
// creation, `name` is a path component, and `group`, `spokeType`, `state`,
// `uniqueId` and `etag` are server-owned. UpdateMaskFromBody builds the mask
// from the body's top-level keys, so any of them left in place turns a
// description change into a rejected call.
var spokeDropOnUpdate = base.DropFieldsOnUpdate(
	"name", "hub", "linkedVpcNetwork", "group", "spokeType", "state", "uniqueId", "etag")

// spokeLinkedVpcNetworkOutputOnly are the members of linkedVpcNetwork the API
// reports but will not accept, and which no forma declares.
//
// A schema `hasProviderDefault` hint only reaches a top-level field, so these
// cannot be waved through the way `state` or `etag` are - Verify compares the
// nested object as a whole and an extra member inside it is a mismatch. They
// are removed on the way in instead.
//
// `vpcNetwork` belongs to the linked-* variants this plugin does not expose;
// the proposed*ExportRanges pair and `producerVpcSpokes` are populated by the
// hub's own accept/reject workflow, not by the spoke's owner.
var spokeLinkedVpcNetworkOutputOnly = []string{
	"vpcNetwork",
	"proposedIncludeExportRanges",
	"proposedExcludeExportRanges",
	"producerVpcSpokes",
}

// expandHubRef turns the short hub id a HubResolvable yields ("my-hub") into
// the full path a spoke's `hub` field requires
// ("projects/{p}/locations/global/hubs/my-hub").
//
// Hubs are always global, so the location is a constant rather than something
// read from the context - a regional segment here would address nothing.
//
// A value that already contains a slash is passed through untouched, so a forma
// may also name a hub in another project explicitly.
func expandHubRef(value, project string) string {
	if value == "" || strings.Contains(value, "/") {
		return value
	}
	return fmt.Sprintf("projects/%s/locations/%s/hubs/%s", project, defaultLocation, value)
}

// shortenHubRef is the exact inverse: it reduces the full path the API reports
// back to the short id the forma declared.
//
// Both halves are mandatory. `hub` is immutable, so expanding on the request
// without shortening on the response leaves the declared value and the stored
// state permanently disagreeing - and every re-apply then plans a replacement
// that the API refuses anyway.
func shortenHubRef(value string) string {
	if i := strings.LastIndex(value, "/"); i >= 0 {
		return value[i+1:]
	}
	return value
}

// spokeRequestTransformer drops the fields a PATCH may not carry and expands
// the hub reference to the full path the API demands.
func spokeRequestTransformer(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	kept, err := spokeDropOnUpdate.Transform(props, ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]interface{}, len(kept))
	for k, v := range kept {
		out[k] = v
	}
	if h, ok := out["hub"].(string); ok {
		out["hub"] = expandHubRef(h, ctx.Project)
	}
	return out, nil
}

// spokeResponseTransformer shortens the spoke's own name and its hub reference
// back to what the forma declared, and strips the output-only members of
// linkedVpcNetwork that a schema hint cannot reach.
func spokeResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	out := base.ShortNameResponseTransformer.Transform(apiResponse, ctx)
	if h, ok := out["hub"].(string); ok {
		out["hub"] = shortenHubRef(h)
	}
	if linked, ok := out["linkedVpcNetwork"].(map[string]interface{}); ok {
		for _, f := range spokeLinkedVpcNetworkOutputOnly {
			delete(linked, f)
		}
	}
	return out
}
