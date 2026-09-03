// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package networkservices

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// endpointPolicyRefCollections maps each EndpointPolicy field that points at a
// Network Security policy to the collection that policy lives in.
//
// All three targets are types this plugin already ships -
// GCP::NetworkSecurity::AuthorizationPolicy, ServerTlsPolicy and
// ClientTlsPolicy - and all three are global, short-name resolvables. So a
// forma names one with `authz.res.name` and gets the short name, while this API
// insists on the full path `projects/{p}/locations/global/{collection}/{name}`:
// verified live, it echoes the full path back on both create and GET.
//
// Hence the two-sided translation. The collection differs per field, which is
// why this is a map rather than a list.
var endpointPolicyRefCollections = map[string]string{
	"authorizationPolicy": "authorizationPolicies",
	"serverTlsPolicy":     "serverTlsPolicies",
	"clientTlsPolicy":     "clientTlsPolicies",
}

// endpointPolicyRequestTransformer expands each policy reference to a full path
// and drops the name on update (it is the path, and UpdateMaskFromBody would
// otherwise put it in the update mask).
//
// Nothing else is dropped, and `type` in particular is not, even though it
// reads like a create-only field. This API's PATCH validates the resource from
// the request body alone: a PATCH that omits `type` answers 400 "EndpointPolicy
// type must be one of SIDECAR_PROXY or GRPC_SERVER, not
// ENDPOINT_POLICY_TYPE_UNSPECIFIED", and one that omits `endpointMatcher`
// answers "MetadataLabelMatcher must be provided", regardless of what the
// stored resource holds and regardless of the update mask. Both are required in
// the schema and so always present in the body, which is what makes an update
// work at all. `type` is genuinely mutable besides - SIDECAR_PROXY to
// GRPC_SERVER was applied live and read back changed.
func endpointPolicyRequestTransformer(
	props map[string]interface{}, ctx base.TransformContext,
) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(props))
	for k, v := range props {
		if k == "name" && ctx.Operation == resource.OperationUpdate {
			continue
		}
		out[k] = v
	}
	for field, collection := range endpointPolicyRefCollections {
		if s, ok := out[field].(string); ok {
			out[field] = expandGlobalRef(s, ctx.Project, collection)
		}
	}
	return out, nil
}

// endpointPolicyResponseTransformer shortens the policy's own name and every
// Network Security reference it reports, so stored state matches what the forma
// declared.
func endpointPolicyResponseTransformer(
	apiResponse map[string]interface{}, _ base.TransformContext,
) map[string]interface{} {
	if name, ok := apiResponse["name"].(string); ok {
		apiResponse["name"] = shortenRef(name)
	}
	for field := range endpointPolicyRefCollections {
		if s, ok := apiResponse[field].(string); ok {
			apiResponse[field] = shortenRef(s)
		}
	}
	return apiResponse
}
