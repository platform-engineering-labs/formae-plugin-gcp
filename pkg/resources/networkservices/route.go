// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package networkservices

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// The four route kinds - httpRoutes, grpcRoutes, tcpRoutes, tlsRoutes - share
// one shape: a name that is the path, a "meshes" listing of full mesh paths,
// and a rules body the API echoes back byte-for-byte. So they share one pair of
// transformers rather than four copies.
//
// What is NOT here, deliberately:
//
//   - "gateways", the other listing a route may attach to. A gateway allocates
//     Envoy proxies and therefore bills, so no gateway could be created to
//     verify the round trip against, and rule 6 says a field that cannot be
//     verified does not get declared. It is absent from the schemas too.
//   - "rules". The API returns rules exactly as sent - every nested object,
//     enum and repeated field came back identical across create, GET and PATCH
//     on all four kinds - so no translation is needed and none is done.
//
// The rules body is also why nothing is dropped on update beyond the name.
// This API's PATCH validates the resource from the request body alone, not from
// the body merged over stored state: a PATCH carrying only "description"
// answers 400 "HTTPRoute must specify at least 1 hostname." even though the
// stored resource has one. Verified on httpRoutes, tcpRoutes and
// endpointPolicies. Every required field must therefore ride along on every
// update, which is exactly what happens when they are non-optional in the
// schema and so always present in the body.

// routeRequestTransformer expands the mesh references to full paths and drops
// the name on update (it is the path, and UpdateMaskFromBody would otherwise
// put it in the update mask).
func routeRequestTransformer(
	props map[string]interface{}, ctx base.TransformContext,
) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(props))
	for k, v := range props {
		if k == "name" && ctx.Operation == resource.OperationUpdate {
			continue
		}
		out[k] = v
	}
	if meshes, ok := out["meshes"]; ok {
		out["meshes"] = expandRefList(meshes, ctx.Project, "meshes")
	}
	return out, nil
}

// routeResponseTransformer shortens the route's own name and every mesh
// reference it reports, so stored state matches what the forma declared.
func routeResponseTransformer(
	apiResponse map[string]interface{}, _ base.TransformContext,
) map[string]interface{} {
	if name, ok := apiResponse["name"].(string); ok {
		apiResponse["name"] = shortenRef(name)
	}
	if meshes, ok := apiResponse["meshes"]; ok {
		apiResponse["meshes"] = shortenRefList(meshes)
	}
	return apiResponse
}
