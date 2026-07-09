// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import "github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"

// routerResponseTransformer extracts the region name from its full URL and drops
// provider-assigned "noise" that no forma declares (so it does not read back as
// perpetual drift).
func routerResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	result := base.RegionResponseTransformer.Transform(apiResponse, ctx)
	delete(result, "encryptedInterconnectRouter")
	// nats are a separate managed resource (RouterNat); the router read must not
	// mirror them or it drifts every time a NAT is attached.
	delete(result, "nats")
	return result
}

// RouterResponseTransformer is the response transformer for Router resources.
var RouterResponseTransformer = base.ResponseTransformerFunc(routerResponseTransformer)
