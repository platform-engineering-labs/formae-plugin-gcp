// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/base"
)

// diskResponseTransformer transforms Disk API responses
// - zone: extracts just the zone name from full URL
// - sourceImage: extracts the relative path (projects/*/global/images/*)
func addressResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	fmt.Println(apiResponse)
	return apiResponse
}
