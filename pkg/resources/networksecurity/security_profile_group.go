// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package networksecurity

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// securityProfileRefFields are the SecurityProfileGroup fields that point at a
// SecurityProfile. Each is a full resource path on the wire.
var securityProfileRefFields = []string{
	"threatPreventionProfile",
	"customInterceptProfile",
	"customMirroringProfile",
	"urlFilteringProfile",
}

// securityProfileGroupDropAlways are fields that must never reach the API in a
// request body. etag and dataPathId are server-owned: dataPathId is assigned at
// create, and echoing a stored etag back on a patch fails the call outright
// with 409 "Provided etag is out of date" as soon as anything else has touched
// the resource. name is a path component, not payload.
var securityProfileGroupDropAlways = map[string]bool{
	"etag":       true,
	"dataPathId": true,
}

// expandSecurityProfileRef turns the short profile name a resolvable yields
// ("my-profile") into the full path the API requires
// ("projects/{p}/locations/global/securityProfiles/my-profile").
//
// Security profiles are always global, so the location is fixed rather than
// taken from the context: a group is global too, and a regional segment here
// would address nothing.
//
// A value that already looks like a path is passed through untouched, so a
// forma may also name a profile in another project explicitly.
func expandSecurityProfileRef(value, project string) string {
	if value == "" || strings.Contains(value, "/") {
		return value
	}
	return fmt.Sprintf("projects/%s/locations/%s/securityProfiles/%s", project, defaultLocation, value)
}

// shortenSecurityProfileRef is the exact inverse: it reduces the full path the
// API reports back to the short name the forma declared.
//
// Both halves are required. Expanding on the request without shortening on the
// response leaves the declared value and the stored state permanently
// disagreeing, and every re-apply then plans a replacement of a resource that
// has not changed.
func shortenSecurityProfileRef(value string) string {
	if i := strings.LastIndex(value, "/"); i >= 0 {
		return value[i+1:]
	}
	return value
}

// securityProfileGroupRequestTransformer expands every profile reference to a
// full path, drops the server-owned fields, and drops the name on update (it is
// the path, and UpdateMaskFromBody would otherwise put it in the mask).
func securityProfileGroupRequestTransformer(
	props map[string]interface{}, ctx base.TransformContext,
) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(props))
	for k, v := range props {
		if securityProfileGroupDropAlways[k] {
			continue
		}
		if k == "name" && ctx.Operation == resource.OperationUpdate {
			continue
		}
		out[k] = v
	}
	for _, f := range securityProfileRefFields {
		if s, ok := out[f].(string); ok {
			out[f] = expandSecurityProfileRef(s, ctx.Project)
		}
	}
	return out, nil
}

// securityProfileGroupResponseTransformer shortens the group's own name and
// every profile reference it reports, so state matches what the forma declared.
func securityProfileGroupResponseTransformer(
	apiResponse map[string]interface{}, _ base.TransformContext,
) map[string]interface{} {
	if name, ok := apiResponse["name"].(string); ok {
		apiResponse["name"] = shortenSecurityProfileRef(name)
	}
	for _, f := range securityProfileRefFields {
		if s, ok := apiResponse[f].(string); ok {
			apiResponse[f] = shortenSecurityProfileRef(s)
		}
	}
	return apiResponse
}
