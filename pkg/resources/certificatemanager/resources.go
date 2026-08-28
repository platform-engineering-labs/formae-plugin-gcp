// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package certificatemanager

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

const (
	CertificateMapResourceType   = "GCP::CertificateManager::CertificateMap"
	DnsAuthorizationResourceType = "GCP::CertificateManager::DnsAuthorization"
	TrustConfigResourceType      = "GCP::CertificateManager::TrustConfig"
)

var certificateManagerRegistry *base.ResourceRegistry

func init() {
	certificateManagerRegistry = base.NewResourceRegistry(
		CertificateManagerAPI, CertificateManagerOperations, CertificateManagerNativeID)

	// All three are location-scoped, take their id as a create-time query
	// parameter, and patch with a query-string field mask - so the generic
	// engine covers them without a custom provisioner.
	err := certificateManagerRegistry.RegisterAll([]base.ResourceDefinition{
		{
			// A certificate map groups the certificates a load balancer serves,
			// selected per hostname by its entries.
			ResourceType: CertificateMapResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "certificateMaps",
				Scope:              &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam:      "certificateMapId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  base.DropFieldsOnUpdate("name"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// A DNS authorization is how Certificate Manager proves control of
			// a domain: it hands back a CNAME to publish, and a managed
			// certificate for that domain cannot be issued without one.
			ResourceType: DnsAuthorizationResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "dnsAuthorizations",
				Scope:              &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam:      "dnsAuthorizationId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  base.DropFieldsOnUpdate("name", "domain"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// A trust config holds the CAs a load balancer will accept client
			// certificates from - the anchor set for mutual TLS.
			ResourceType: TrustConfigResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "trustConfigs",
				Scope:              &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam:      "trustConfigId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  base.DropFieldsOnUpdate("name"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
	})
	if err != nil {
		panic(err)
	}
}
