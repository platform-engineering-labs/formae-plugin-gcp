// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// sslCertificateRequestTransformer re-nests the flattened schema fields into the
// API's wrapper objects. The schema flattens two API structures because the base
// Resource class reserves the `managed` name and nested write-only keys are
// awkward to model:
//   - MANAGED:      managedDomains        -> managed { domains: [...] }
//   - SELF_MANAGED: certificate/privateKey -> selfManaged { certificate, privateKey }
//
// GCP rejects a self-managed cert whose details are sent at the top level
// ("Self-managed certificate details must be specified if type = SELF_MANAGED").
func sslCertificateRequestTransformer(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		switch k {
		case "managedDomains", "certificate", "privateKey":
			continue // re-nested below
		default:
			body[k] = v
		}
	}
	if domains, ok := props["managedDomains"]; ok {
		body["managed"] = map[string]interface{}{"domains": domains}
	}
	selfManaged := make(map[string]interface{})
	if cert, ok := props["certificate"]; ok {
		selfManaged["certificate"] = cert
	}
	if key, ok := props["privateKey"]; ok {
		selfManaged["privateKey"] = key
	}
	if len(selfManaged) > 0 {
		body["selfManaged"] = selfManaged
	}
	return body, nil
}

// sslCertificateResponseTransformer is the inverse: it lifts the API's wrapper
// objects back to the flattened schema fields and drops the wrappers so the read
// state round-trips against the desired state (privateKey is write-only and never
// returned by the API).
func sslCertificateResponseTransformer(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
	result := make(map[string]interface{}, len(apiResponse))
	for k, v := range apiResponse {
		result[k] = v
	}
	if managed, ok := apiResponse["managed"].(map[string]interface{}); ok {
		if domains, ok := managed["domains"]; ok {
			result["managedDomains"] = domains
		}
		delete(result, "managed")
	}
	if selfManaged, ok := apiResponse["selfManaged"].(map[string]interface{}); ok {
		if cert, ok := selfManaged["certificate"]; ok {
			result["certificate"] = cert
		}
		delete(result, "selfManaged")
	}
	return result
}
