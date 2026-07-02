// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// sslCertificateRequestTransformer nests the flattened `managedDomains` schema
// field into the API's `managed: { domains: [...] }` structure. The schema
// cannot expose a field literally named `managed` because the base Resource
// class reserves it, so it is flattened in the schema and re-nested here.
func sslCertificateRequestTransformer(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		if k == "managedDomains" {
			continue
		}
		body[k] = v
	}
	if domains, ok := props["managedDomains"]; ok {
		body["managed"] = map[string]interface{}{"domains": domains}
	}
	return body, nil
}
