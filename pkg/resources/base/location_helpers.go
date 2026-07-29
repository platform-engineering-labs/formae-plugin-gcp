// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package base

import (
	"fmt"
	"strings"
)

// Helpers shared by location-scoped GCP REST APIs whose resources live under
//   /projects/{project}/locations/{location}/{collection}/{name}
// and whose create/get/delete are synchronous (e.g. Cloud Scheduler, Cloud
// Tasks, Monitoring). A new such service needs only these three wired into its
// APIConfig/OperationConfig plus ShortNameResponseTransformer — no custom code.

// LocationPathBuilder builds "/projects/{project}/locations/{location}/{resourceType}[/{name}]".
func LocationPathBuilder(ctx PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s/%s", ctx.Project, ctx.Location, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// LocationNativeIDExtractor returns the full resource path from a create/read
// response. These APIs echo the fully-qualified name in "name"
// (e.g. "projects/p/locations/l/jobs/x"); fall back to building it from context
// when the response omits it.
func LocationNativeIDExtractor(response map[string]interface{}, ctx PathContext) string {
	if name, ok := response["name"].(string); ok && name != "" {
		if i := strings.Index(name, "projects/"); i >= 0 {
			return name[i:]
		}
	}
	if ctx.ResourceName != "" {
		return fmt.Sprintf("projects/%s/locations/%s/%s/%s",
			ctx.Project, ctx.Location, ctx.ResourceType, ctx.ResourceName)
	}
	return ""
}

// FullResourceNameExpander rewrites a short, user-declared "name" into the
// fully-qualified GCP resource name required in the request body of
// location-scoped create calls. Idempotent: an already-qualified name (contains
// "/") or an absent name is left untouched.
func FullResourceNameExpander() RequestTransformerFunc {
	return func(props map[string]interface{}, ctx TransformContext) (map[string]interface{}, error) {
		name, ok := props["name"].(string)
		if !ok || name == "" || strings.Contains(name, "/") {
			return props, nil
		}
		out := make(map[string]interface{}, len(props))
		for k, v := range props {
			out[k] = v
		}
		out["name"] = fmt.Sprintf("projects/%s/locations/%s/%s/%s",
			ctx.Project, ctx.Location, ctx.ResourceType, name)
		return out, nil
	}
}
