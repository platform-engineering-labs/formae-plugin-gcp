// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// vpnTunnelResponseTransformer drops the secret material the API echoes back.
//
// vpnTunnels.get returns "sharedSecret" as a masked placeholder plus a
// "sharedSecretHash". Keeping either would store a value that is not the
// authored secret, which fails verification of the opaque input ("Opaque value
// sharedSecret digest does not match the authored secret") and would put a
// pointless hash in state. sharedSecret is declared writeOnly, so dropping it
// on read is the behaviour callers expect — the same treatment
// sslCertificate.privateKey gets, except there the API never returns it at all.
func vpnTunnelResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	delete(apiResponse, "sharedSecret")
	delete(apiResponse, "sharedSecretHash")

	// Regional resources report "region" as a full URL.
	if region, ok := apiResponse["region"].(string); ok && region != "" {
		apiResponse["region"] = base.ExtractLastSegment(region)
	}

	return apiResponse
}
