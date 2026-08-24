// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
)

// regionDiskRequestTransformer expands the short forms callers write into the
// full URLs the API demands. Verified against the live API: a bare
// "pd-balanced" is rejected with "Invalid value for field 'resource.type':
// 'pd-balanced'. The URL is malformed.", and replicaZones behaves the same way.
func regionDiskRequestTransformer(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	if diskType := utils.GetString(props, "type"); diskType != "" {
		if !strings.HasPrefix(diskType, "projects/") && !strings.Contains(diskType, "/") {
			props["type"] = fmt.Sprintf("projects/%s/regions/%s/diskTypes/%s",
				ctx.Project, ctx.Region, diskType)
		}
	}

	if zones, ok := props["replicaZones"].([]interface{}); ok {
		expanded := make([]interface{}, len(zones))
		for i, z := range zones {
			zone, ok := z.(string)
			if !ok {
				expanded[i] = z
				continue
			}
			if strings.Contains(zone, "/") {
				expanded[i] = zone
				continue
			}
			expanded[i] = fmt.Sprintf(
				"https://www.googleapis.com/compute/v1/projects/%s/zones/%s", ctx.Project, zone)
		}
		props["replicaZones"] = expanded
	}

	return props, nil
}

// regionDiskResponseTransformer shortens the URLs the API echoes back so the
// stored state matches the declared forma, and restores "project", which the
// response does not carry.
func regionDiskResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	for _, field := range []string{"region", "type"} {
		if v, ok := apiResponse[field].(string); ok && v != "" {
			apiResponse[field] = base.ExtractLastSegment(v)
		}
	}

	if zones, ok := apiResponse["replicaZones"].([]interface{}); ok {
		short := make([]interface{}, len(zones))
		for i, z := range zones {
			if zone, ok := z.(string); ok {
				short[i] = base.ExtractLastSegment(zone)
			} else {
				short[i] = z
			}
		}
		apiResponse["replicaZones"] = short
	}

	if sourceImage, ok := apiResponse["sourceImage"].(string); ok && sourceImage != "" {
		apiResponse["sourceImage"] = extractProjectsPath(sourceImage)
	}

	apiResponse["project"] = ctx.Project

	return apiResponse
}
