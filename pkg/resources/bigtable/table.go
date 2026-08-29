// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigtable

import (
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
)

// tableBodyBuilder builds the request body for creating a Bigtable table
func tableBodyBuilder(props map[string]interface{}) (map[string]interface{}, error) {
	body := make(map[string]interface{})

	// Column families
	if columnFamilies := utils.GetObject(props, "columnFamilies"); columnFamilies != nil {
		families := make(map[string]interface{})
		for familyName, familyData := range columnFamilies {
			if familyMap, ok := familyData.(map[string]interface{}); ok {
				family := make(map[string]interface{})

				// GC rule (garbage collection policy)
				if gcRule := utils.GetObject(familyMap, "gcRule"); gcRule != nil {
					family["gcRule"] = buildGCRuleForAPI(gcRule)
				}

				families[familyName] = family
			}
		}
		if len(families) > 0 {
			body["columnFamilies"] = families
		}
	}

	// Granularity (for table splitting)
	if granularity := utils.GetString(props, "granularity"); granularity != "" {
		body["granularity"] = granularity
	}

	// Initial splits
	if initialSplits := utils.GetArray(props, "initialSplits"); initialSplits != nil {
		splits := make([]map[string]interface{}, 0)
		for _, split := range initialSplits {
			if splitMap, ok := split.(map[string]interface{}); ok {
				splits = append(splits, splitMap)
			}
		}
		if len(splits) > 0 {
			body["initialSplits"] = splits
		}
	}

	return body, nil
}

// buildGCRuleForAPI builds a GC rule for the API request format
func buildGCRuleForAPI(gcRule map[string]interface{}) map[string]interface{} {
	rule := make(map[string]interface{})

	// MaxNumVersions
	if maxVersions := utils.GetInt32(gcRule, "maxNumVersions"); maxVersions > 0 {
		rule["maxNumVersions"] = maxVersions
	}

	// MaxAge (duration string like "72h" or "3d")
	if maxAge := utils.GetString(gcRule, "maxAge"); maxAge != "" {
		rule["maxAge"] = maxAge
	}

	// Union (logical OR of multiple rules)
	if union := utils.GetArray(gcRule, "union"); union != nil {
		unionRules := make([]map[string]interface{}, 0)
		for _, u := range union {
			if uMap, ok := u.(map[string]interface{}); ok {
				unionRules = append(unionRules, buildGCRuleForAPI(uMap))
			}
		}
		if len(unionRules) > 0 {
			rule["union"] = map[string]interface{}{
				"rules": unionRules,
			}
		}
	}

	// Intersection (logical AND of multiple rules)
	if intersection := utils.GetArray(gcRule, "intersection"); intersection != nil {
		intersectionRules := make([]map[string]interface{}, 0)
		for _, i := range intersection {
			if iMap, ok := i.(map[string]interface{}); ok {
				intersectionRules = append(intersectionRules, buildGCRuleForAPI(iMap))
			}
		}
		if len(intersectionRules) > 0 {
			rule["intersection"] = map[string]interface{}{
				"rules": intersectionRules,
			}
		}
	}

	return rule
}

// tableResponseTransformer puts back the short name and the instance.
//
// The Bigtable Admin API reports a table's "name" as the full path
// "projects/{p}/instances/{i}/tables/{t}", while a forma declares the short id
// and the instance separately. Without this every read differs from what was
// declared, so a table reported drift the moment it was created - and the
// instance, which lives only in the path, went missing entirely.
//
// Instance and Cluster already had transformers of their own; Table was left
// with the generic one, which only fills in the project.
func tableResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	out := base.AddProjectResponseTransformer.Transform(apiResponse, ctx)

	name, ok := out["name"].(string)
	if !ok {
		return out
	}
	parts := strings.Split(name, "/")
	// projects/{project}/instances/{instance}/tables/{table}
	if len(parts) == 6 && parts[2] == "instances" && parts[4] == "tables" {
		out["instance"] = parts[3]
		out["name"] = parts[5]
	}
	return out
}
