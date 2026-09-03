// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package networksecurity

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// securityProfileDropAlways names the fields that must never be sent back.
// etag is server-owned, and replaying a stored one on a patch fails with 409
// "Provided etag is out of date" the moment anything else has touched the
// profile.
var securityProfileDropAlways = map[string]bool{
	"etag": true,
}

// securityProfileDropOnUpdate names the fields a PATCH may not carry. The
// update mask is built from the body, so leaving either in place puts it in the
// mask: name is the path rather than payload, and type is fixed at creation.
var securityProfileDropOnUpdate = map[string]bool{
	"name": true,
	"type": true,
}

// securityProfileRequestTransformer drops the server-owned and immutable fields
// according to the operation.
func securityProfileRequestTransformer(
	props map[string]interface{}, ctx base.TransformContext,
) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(props))
	for k, v := range props {
		if securityProfileDropAlways[k] {
			continue
		}
		if ctx.Operation == resource.OperationUpdate && securityProfileDropOnUpdate[k] {
			continue
		}
		out[k] = v
	}
	return out, nil
}

// securityProfileResponseTransformer shortens the full-path name and strips the
// one output-only field that lives too deep for a schema hint to reach.
//
// threatPreventionProfile.threatOverrides[].type is classified output-only by
// the API: it is inferred from threatId and reported back on every read, but a
// forma cannot declare it. Schema hints such as hasProviderDefault apply only
// to top-level fields, so there is no way to annotate it — left in place, Verify
// sees a property the schema never declared and rejects the read.
func securityProfileResponseTransformer(
	apiResponse map[string]interface{}, _ base.TransformContext,
) map[string]interface{} {
	if name, ok := apiResponse["name"].(string); ok {
		apiResponse["name"] = shortenSecurityProfileRef(name)
	}

	tpp, ok := apiResponse["threatPreventionProfile"].(map[string]interface{})
	if !ok {
		return apiResponse
	}
	overrides, ok := tpp["threatOverrides"].([]interface{})
	if !ok {
		return apiResponse
	}
	for _, o := range overrides {
		if override, ok := o.(map[string]interface{}); ok {
			delete(override, "type")
		}
	}
	return apiResponse
}
