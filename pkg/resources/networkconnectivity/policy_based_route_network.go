// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package networkconnectivity

import (
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// computeSelfLinkPrefix is the form Compute reports a network in, and so the
// form a reference to one resolves to inside a forma.
const computeSelfLinkPrefix = "https://www.googleapis.com/compute/v1/"

// Policy-based routes are the odd one out in this API. An internal range and a
// service connection policy both accept a network self link and echo it back
// unchanged, so a reference to a network round-trips on its own. A policy-based
// route rejects the same value:
//
//	invalid argument: network uri "https://www.googleapis.com/compute/v1/projects/p/global/networks/n"
//	is not in the form of projects/my-project/global/networks/my-network
//
// and, given the relative form, reports the relative form back. Left alone that
// is drift on every re-apply: the forma holds a self link and state holds a
// path, on an immutable field, so the plan is a replacement of the route
// already in place.
//
// The two transformers below close that gap in the plugin rather than in every
// fixture: the request is cut down to what the API accepts, and the response is
// expanded back to what the forma declared. A forma that names the network as a
// path already is left alone in both directions.

// policyBasedRouteRequestTransformer cuts a network self link down to the
// "projects/{p}/global/networks/{n}" form the API demands.
func policyBasedRouteRequestTransformer(body map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(body))
	for k, v := range body {
		out[k] = v
	}
	if n, ok := out["network"].(string); ok && n != "" {
		if i := strings.Index(n, "projects/"); i >= 0 {
			out["network"] = n[i:]
		}
	}
	return out, nil
}

// policyBasedRouteResponseTransformer expands the reported path back to a self
// link, so it matches the reference a forma resolved.
func policyBasedRouteResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	out := base.ShortNameResponseTransformer.Transform(apiResponse, ctx)
	if n, ok := out["network"].(string); ok && strings.HasPrefix(n, "projects/") {
		out["network"] = computeSelfLinkPrefix + n
	}
	return out
}
