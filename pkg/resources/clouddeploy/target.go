// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package clouddeploy

import (
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// Cloud Deploy names its own resources in two different forms in the same API,
// which is the one thing about it a transformer has to know:
//
//   - A pipeline stage names a target by *short id*. The full path is not
//     merely unnecessary there, it is rejected outright:
//     "\"projects/p/locations/l/targets/t\" is not a valid resource ID for
//     resource type \"stage.targetId\"" - and the create fails as a done
//     operation carrying that error. So stage.targetId is passed through
//     untouched; a Target resolvable yields exactly the short name it wants.
//   - A custom target names its CustomTargetType by *full path*
//     ("projects/{p}/locations/{l}/customTargetTypes/{id}"), which is what the
//     API documents as required.
//
// The second is the form a resolvable cannot produce - it yields a declared
// property, which for a CustomTargetType is its short name - so the request
// expands and the response shortens, and the two meet at the form the forma
// used. Doing only half of that is worse than doing neither: declared state and
// stored state would disagree permanently on a field, and every re-apply would
// plan a change that changes nothing.
//
// Both forms were observed live: the API accepts a short id here as well and
// echoes back whatever it was sent, so the expansion is about sending the
// documented form rather than about being accepted.

// customTargetTypeCollection is the collection segment of a CustomTargetType
// path.
const customTargetTypeCollection = "customTargetTypes"

// expandCustomTargetTypeRef turns the short name a resolvable yields into the
// full path the API documents. A value that already carries a path, or a
// context with no project, is left untouched.
func expandCustomTargetTypeRef(value, project, location string) string {
	if value == "" || strings.Contains(value, "/") || project == "" || location == "" {
		return value
	}
	return "projects/" + project + "/locations/" + location + "/" + customTargetTypeCollection + "/" + value
}

// shortenRef is the exact inverse: it reduces a full path to its last segment.
func shortenRef(value string) string {
	if i := strings.LastIndex(value, "/"); i >= 0 {
		return value[i+1:]
	}
	return value
}

// transformCustomTarget applies fn to customTarget.customTargetType in place.
func transformCustomTarget(props map[string]interface{}, fn func(string) string) {
	ct, ok := props["customTarget"].(map[string]interface{})
	if !ok {
		return
	}
	if s, ok := ct["customTargetType"].(string); ok {
		ct["customTargetType"] = fn(s)
	}
}

// targetLocation is the location segment a Target's own references live in. A
// target and the custom target type it names are both regional and must share
// the region, so the target's own location is the right one.
func targetLocation(ctx base.TransformContext) string {
	if ctx.Location != "" {
		return ctx.Location
	}
	return ctx.Region
}

// targetRequestTransformer expands the custom target type reference and drops
// the name on update: it is the path, and UpdateMaskFromBody would otherwise
// put it in the mask.
func targetRequestTransformer(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	out, err := base.DropFieldsOnUpdate("name").Transform(props, ctx)
	if err != nil {
		return nil, err
	}
	transformCustomTarget(out, func(s string) string {
		return expandCustomTargetTypeRef(s, ctx.Project, targetLocation(ctx))
	})
	return out, nil
}

// targetResponseTransformer shortens the custom target type reference the API
// reports, then shortens the target's own full-path name.
var targetResponseTransformer = &base.CompositeResponseTransformer{
	Transformers: []base.ResponseTransformer{
		base.ResponseTransformerFunc(
			func(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
				transformCustomTarget(apiResponse, shortenRef)
				return apiResponse
			}),
		base.ShortNameResponseTransformer,
	},
}
