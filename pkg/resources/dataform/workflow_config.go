// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dataform

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// unspecifiedQueryPriority is what the API reports for a workflow config whose
// invocationConfig never named one. Verified live: a create sending
// invocationConfig with nothing but a serviceAccount answers with
// {"serviceAccount": "...", "queryPriority": "QUERY_PRIORITY_UNSPECIFIED"}.
const unspecifiedQueryPriority = "QUERY_PRIORITY_UNSPECIFIED"

// transformContextLocation is the location a workflow config's release-config
// path is built from. TransformContext carries both, and a Dataform resource is
// always regional, so either will do - Location first because that is what the
// target config sets and what the path builder uses.
func transformContextLocation(ctx base.TransformContext) string {
	if ctx.Location != "" {
		return ctx.Location
	}
	return ctx.Region
}

// expandReleaseConfig rewrites the declared release config into the full path
// the API demands: projects/*/locations/*/repositories/*/releaseConfigs/*.
//
// This is the half of the round trip that has to exist on the request side. A
// resolvable yields only the release config's short name, and the API stores
// and reports whatever full path it was given, so declaring the short form and
// sending it unchanged would be rejected - and expanding it here without
// shortening it in the response transformer would be worse than doing neither:
// declared state and stored state would disagree permanently on a field that is
// mutable, so every reconcile would plan an update that changed nothing.
//
// The release config is assumed to live in the same repository as the workflow
// config, which is the shape the API's own path implies and the only one a
// single resolvable can express. A value already carrying "/" is left alone, so
// a full path written out by hand still works.
func expandReleaseConfig(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	rc, ok := props["releaseConfig"].(string)
	if !ok || rc == "" || strings.Contains(rc, "/") {
		return props, nil
	}
	if ctx.Project == "" || transformContextLocation(ctx) == "" || ctx.ParentResource == "" {
		return nil, fmt.Errorf(
			"cannot expand releaseConfig %q: project, location and repository are all required", rc)
	}
	out := make(map[string]interface{}, len(props))
	for k, v := range props {
		out[k] = v
	}
	out["releaseConfig"] = fmt.Sprintf("projects/%s/locations/%s/repositories/%s/releaseConfigs/%s",
		ctx.Project, transformContextLocation(ctx), ctx.ParentResource, rc)
	return out, nil
}

// workflowConfigRequestTransformer expands the release config reference, drops
// the repository from every body, and drops the create-only fields from update
// bodies.
//
//   - "repository" is a path component. The API answers a body carrying it with
//     400 "Unknown name \"repository\": Cannot find field".
//   - "name" survives a create - base reads the id out of it for
//     ?workflowConfigId= - and has to leave an update so it does not enter the
//     mask as a rename.
//   - "invocationConfig" is immutable, and the refusal is by mask rather than by
//     value: PATCH ?updateMask=invocationConfig answers 400 "Request
//     update_mask contains immutable fields: [invocation_config]" even when the
//     object is unchanged. Nothing in the API's discovery document says so -
//     the field is documented as plain "Optional" - which is why this was
//     established against the live API. Since a reconcile PATCH sends every
//     declared field, leaving it in would fail every update of releaseConfig,
//     cronSchedule, timeZone or disabled.
//
// The expansion runs first so it sees the property before anything is removed.
var workflowConfigRequestTransformer = &base.CompositeRequestTransformer{
	Transformers: []base.RequestTransformer{
		base.RequestTransformerFunc(expandReleaseConfig),
		base.DropFields("repository"),
		base.DropFieldsOnUpdate("name", "invocationConfig"),
	},
}

// workflowConfigResponseTransformer shortens the release config reference back
// to the name a forma declared, puts the repository back, and drops what the
// API owns.
//
// Shortening the release config is the other half of expandReleaseConfig; see
// there for why one side alone is worse than neither.
//
// invocationConfig.queryPriority is unset back to absent when the API reports
// it as QUERY_PRIORITY_UNSPECIFIED. It is nested, so a hasProviderDefault hint
// in the schema cannot reach it, and it materialises on any workflow config
// that declared an invocationConfig without a priority - which every one of
// them must, because the API refuses a create with no
// invocationConfig.serviceAccount ("Service account must be set when strict act
// as checks are enabled").
//
// Dropped top-level:
//
//   - createTime, updateTime, internalMetadata - server bookkeeping.
//   - recentScheduledExecutionRecords - the last ten scheduled invocation
//     attempts. Output-only and it grows on its own.
var workflowConfigResponseTransformer = &base.CompositeResponseTransformer{
	Transformers: []base.ResponseTransformer{
		restoreRepository,
		base.ResponseTransformerFunc(
			func(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
				if rc, ok := apiResponse["releaseConfig"].(string); ok {
					if i := strings.LastIndex(rc, "/"); i >= 0 {
						apiResponse["releaseConfig"] = rc[i+1:]
					}
				}
				if ic, ok := apiResponse["invocationConfig"].(map[string]interface{}); ok {
					if p, ok := ic["queryPriority"].(string); ok && p == unspecifiedQueryPriority {
						delete(ic, "queryPriority")
					}
				}
				return apiResponse
			}),
		dropFields(
			"createTime",
			"updateTime",
			internalMetadata,
			"recentScheduledExecutionRecords",
		),
		base.ShortNameResponseTransformer,
	},
}
