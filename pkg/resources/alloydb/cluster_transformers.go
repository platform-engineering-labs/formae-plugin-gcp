// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package alloydb

import (
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// networkCanonicalizeTransformer rewrites the project segment of
// networkConfig.network back to the declared project ID.
//
// Users declare the network as "projects/<projectID>/global/networks/<net>",
// but on read-back GCP canonicalizes it to "projects/<projectNumber>/...".
// That mismatch fails Verify. Rewrite the segment after "projects/" to
// ctx.Project so read matches the declaration. No-op when absent or already
// short.
var networkCanonicalizeTransformer = base.ResponseTransformerFunc(
	func(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
		nc, ok := apiResponse["networkConfig"].(map[string]interface{})
		if !ok {
			return apiResponse
		}
		network, ok := nc["network"].(string)
		if !ok || network == "" {
			return apiResponse
		}
		const prefix = "projects/"
		if !strings.HasPrefix(network, prefix) {
			return apiResponse
		}
		// network == "projects/<num>/global/networks/<net>"
		rest := network[len(prefix):]
		if i := strings.Index(rest, "/"); i >= 0 {
			nc["network"] = prefix + ctx.Project + rest[i:]
		}
		return apiResponse
	})

// clusterResponseTransformer canonicalizes the network project segment, then
// shortens the full-path name to its last segment.
var clusterResponseTransformer = &base.CompositeResponseTransformer{
	Transformers: []base.ResponseTransformer{
		networkCanonicalizeTransformer,
		base.ShortNameResponseTransformer,
	},
}
