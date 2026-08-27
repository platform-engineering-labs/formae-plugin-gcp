// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package sql

import (
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// SQLSslCertOperations - a client certificate is the one sqladmin resource that
// comes back whole from its own create: the insert response carries the
// certificate alongside the Operation, so there is nothing to poll.
var SQLSslCertOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractSslCertNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// extractSslCertNativeID addresses a certificate by its server-generated
// sha1Fingerprint, which is what get and delete take as their path segment. A
// forma declares only `commonName`, so unlike every other resource here the
// native ID cannot be built from a declared property.
//
// The fingerprint arrives in two shapes: nested under clientCert.certInfo on
// insert, and at the top level on get and in a list item.
func extractSslCertNativeID(response map[string]interface{}, ctx base.PathContext) string {
	fingerprint := sslCertFingerprint(response)
	if fingerprint == "" || ctx.ParentResource == "" {
		return ""
	}
	return fmt.Sprintf("projects/%s/instances/%s/sslCerts/%s",
		ctx.Project, ctx.ParentResource, fingerprint)
}

func sslCertFingerprint(response map[string]interface{}) string {
	if fp, ok := response["sha1Fingerprint"].(string); ok && fp != "" {
		return fp
	}
	if clientCert, ok := response["clientCert"].(map[string]interface{}); ok {
		if certInfo, ok := clientCert["certInfo"].(map[string]interface{}); ok {
			if fp, ok := certInfo["sha1Fingerprint"].(string); ok {
				return fp
			}
		}
	}
	return ""
}

// sslCertResponseTransformer lifts the certificate out of the insert response's
// wrapper and drops the one field that must never reach stored state.
//
// insert answers {"clientCert": {"certInfo": {...}, "certPrivateKey": "..."},
// "serverCaCert": {...}, "operation": {...}} while get answers the certInfo
// object directly, so reads would otherwise disagree with creates on every
// field. certPrivateKey is returned exactly once and never again: keeping it
// would both persist a private key and guarantee drift on the next read.
func sslCertResponseTransformer(props map[string]interface{}, _ base.TransformContext) map[string]interface{} {
	cert := props
	if clientCert, ok := props["clientCert"].(map[string]interface{}); ok {
		if certInfo, ok := clientCert["certInfo"].(map[string]interface{}); ok {
			cert = certInfo
		}
	}

	out := make(map[string]interface{}, len(cert))
	for k, v := range cert {
		switch k {
		case "certPrivateKey", "kind", "selfLink", "project":
			continue
		}
		out[k] = v
	}
	return out
}
