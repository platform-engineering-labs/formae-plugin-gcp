// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package certificateauthority implements GCP Certificate Authority Service
// (privateca) resources.
package certificateauthority

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const CaPoolResourceType = "GCP::CertificateAuthority::CaPool"

var certificateAuthorityRegistry *base.ResourceRegistry

func init() {
	certificateAuthorityRegistry = base.NewResourceRegistry(
		CertificateAuthorityAPI, CertificateAuthorityOperations, CertificateAuthorityNativeID)

	// ponytail: Update deferred (as artifactregistry/DNS/scheduler do) until
	// PATCH is verified live. create/delete are the async paths this batch proves out.
	err := certificateAuthorityRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: CaPoolResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "caPools",
				Scope:         &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam: "caPoolId", // id goes in ?caPoolId=, not the body
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
	})
	if err != nil {
		panic(err)
	}
}
