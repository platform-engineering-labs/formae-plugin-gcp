// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package sql

import (
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
)

// SQLSslCertOperations - a client certificate is the one sqladmin resource whose
// create answers with the resource itself, carried alongside the Operation. It
// is tempting to call that synchronous, and this code did: the certificate is
// usable the moment insert returns, so there is nothing to wait for.
//
// That is wrong, and Cloud SQL says so with a 409. Operations are serialised
// per instance, so reporting the create done while its operation is still
// running means the next mutation - the conformance Destroy, say - is issued
// into a busy queue and answers "Operation failed because another operation was
// already in progress". The certificate has to be polled like everything else;
// only its native ID comes from somewhere unusual.
var SQLSslCertOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   extractSslCertOperationID,
	OperationURLBuilder:    SQLOperations.OperationURLBuilder,
	NativeIDExtractor:      extractSslCertNativeID,
	OperationStatusChecker: SQLOperations.OperationStatusChecker,
	RetryableError:         isRetryableSQLError,
}

// extractSslCertOperationID digs the operation out of the insert response,
// where it sits beside the certificate rather than at the top level as it does
// for every other sqladmin mutation.
func extractSslCertOperationID(response map[string]interface{}) string {
	if operation, ok := response["operation"].(map[string]interface{}); ok {
		return utils.GetString(operation, "name")
	}
	return utils.GetString(response, "name")
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
