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

// diskRequestTransformer transforms disk creation requests
// - type: converts short form (pd-standard) to full URL (projects/{project}/zones/{zone}/diskTypes/pd-standard)
func diskRequestTransformer(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	// Transform type field to full URL if it's a short form
	if diskType := utils.GetString(props, "type"); diskType != "" {
		// If not already in full path format, convert it
		if !strings.HasPrefix(diskType, "projects/") && !strings.HasPrefix(diskType, "zones/") {
			props["type"] = fmt.Sprintf("projects/%s/zones/%s/diskTypes/%s", ctx.Project, ctx.Zone, diskType)
		}
	}

	return props, nil
}

// diskResponseTransformer transforms Disk API responses
// - zone: extracts just the zone name from full URL
// - sourceImage: extracts the relative path (projects/*/global/images/*)
func diskResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {

	// Transform zone - extract last segment
	if zone, ok := apiResponse["zone"].(string); ok && zone != "" {
		apiResponse["zone"] = base.ExtractLastSegment(zone)
	}

	// Transform zone - extract last segment
	if diskType, ok := apiResponse["type"].(string); ok && diskType != "" {
		apiResponse["type"] = base.ExtractLastSegment(diskType)
	}

	// Transform sourceImage - extract relative path starting from "projects/"
	if sourceImage, ok := apiResponse["sourceImage"].(string); ok && sourceImage != "" {
		apiResponse["sourceImage"] = extractProjectsPath(sourceImage)
	}

	apiResponse["physicalBlockSizeBytes"] = utils.GetInt64(apiResponse, "physicalBlockSizeBytes")
	apiResponse["project"] = ctx.Project

	return apiResponse
}

// extractProjectsPath extracts the path starting from "projects/" from a full URL
// e.g., "https://www.googleapis.com/compute/v1/projects/debian-cloud/global/images/debian-12-bookworm-v20251111"
// -> "projects/debian-cloud/global/images/debian-12-bookworm-v20251111"
func extractProjectsPath(url string) string {
	const projectsPrefix = "projects/"
	idx := strings.Index(url, projectsPrefix)
	if idx != -1 {
		return url[idx:]
	}
	// If no "projects/" found, return the original
	return url
}
