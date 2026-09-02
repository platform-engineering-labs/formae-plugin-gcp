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

	CertificateResourceType         = "GCP::CertificateManager::Certificate"
	CertificateMapEntryResourceType = "GCP::CertificateManager::CertificateMapEntry"
)

var certificateManagerRegistry *base.ResourceRegistry

func init() {
	certificateManagerRegistry = base.NewResourceRegistry(
		CertificateManagerAPI, CertificateManagerOperations, CertificateManagerNativeID)

	// All three are global, take their id as a create-time query parameter, and
	// patch with a query-string field mask - so the generic engine covers them
	// without a custom provisioner. They carry no Scope: ScopeLocationBased
	// would make List return nothing whenever the target declares no location,
	// and the path builder pins "global" regardless.
	err := certificateManagerRegistry.RegisterAll([]base.ResourceDefinition{
		{
			// A certificate map groups the certificates a load balancer serves,
			// selected per hostname by its entries.
			ResourceType: CertificateMapResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "certificateMaps",
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
				CreateIDParam:      "trustConfigId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  base.DropFieldsOnUpdate("name"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// A TLS certificate a load balancer can serve. A managed one is
			// obtained and renewed by Google against a DnsAuthorization and
			// carries no private key, which is the kind a repository can
			// describe; selfManaged is the other half of the type.
			//
			// Only description and labels may be patched - the certificate
			// itself is fixed at creation - so everything else is dropped on
			// update rather than entering the mask.
			ResourceType: CertificateResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "certificates",
				CreateIDParam:      "certificateId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  base.RequestTransformerFunc(certificateRequestTransformer),
			ResponseTransformer: certificateResponseTransformer,
		},
		{
			// One row of a certificate map: which certificates to serve for a
			// hostname. Nested under its map, which is a path component rather
			// than a body field.
			ResourceType: CertificateMapEntryResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "certificateMapEntries",
				CreateIDParam: "certificateMapEntryId",
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "certificateMaps",
					PropertyName:   "certificateMap",
					RequiresParent: true,
				},
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  base.RequestTransformerFunc(certificateMapEntryRequestTransformer),
			ResponseTransformer: certificateMapEntryResponseTransformer,
		},
	})
	if err != nil {
		panic(err)
	}
}
