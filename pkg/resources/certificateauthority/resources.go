// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package certificateauthority implements GCP Certificate Authority Service
// (privateca) resources.
package certificateauthority

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const (
	CaPoolResourceType               = "GCP::CertificateAuthority::CaPool"
	CertificateAuthorityResourceType = "GCP::CertificateAuthority::CertificateAuthority"
	CertificateTemplateResourceType  = "GCP::CertificateAuthority::CertificateTemplate"
)

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
		{
			// The CA that actually signs. A pool without one is inert.
			ResourceType: CertificateAuthorityResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "certificateAuthorities",
				Scope:        &base.ScopeConfig{Type: base.ScopeLocationBased},
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "caPools",
					PropertyName:   "caPool",
					RequiresParent: true,
				},
				CreateIDParam: "certificateAuthorityId",
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			RequestTransformer:  base.RequestTransformerFunc(dropCAPathFields),
			ResponseTransformer: base.ResponseTransformerFunc(caResponseTransformer),
		},
		{
			// A reusable issuance policy. Location-scoped, not pool-scoped, and
			// free - it issues nothing on its own.
			ResourceType: CertificateTemplateResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "certificateTemplates",
				Scope:         &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam: "certificateTemplateId",
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			RequestTransformer:  base.RequestTransformerFunc(dropCAPathFields),
			ResponseTransformer: locationResponseTransformer("certificateTemplates"),
		},
	})
	if err != nil {
		panic(err)
	}

	// Re-register the CA behind a provisioner that overrides Delete. Everything
	// else stays config-driven; only Delete has to differ, because a plain
	// DELETE leaves the CA tombstoned for 30 days. See ca_delete.go.
	registry.Register(
		CertificateAuthorityResourceType,
		certificateAuthorityRegistry.Definitions[CertificateAuthorityResourceType].Operations,
		func(cfg *config.Config) prov.Provisioner {
			def := certificateAuthorityRegistry.Definitions[CertificateAuthorityResourceType]
			return &caProvisioner{
				BaseResource: &base.BaseResource{
					Config:              cfg,
					APIConfig:           CertificateAuthorityAPI,
					OperationConfig:     CertificateAuthorityOperations,
					ResourceConfig:      def.ResourceConfig,
					NativeIDConfig:      CertificateAuthorityNativeID,
					RequestTransformer:  def.RequestTransformer,
					ResponseTransformer: def.ResponseTransformer,
				},
			}
		},
	)
}
