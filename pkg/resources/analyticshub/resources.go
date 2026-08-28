// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package analyticshub implements GCP Analytics Hub resources.
package analyticshub

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const (
	DataExchangeResourceType  = "GCP::AnalyticsHub::DataExchange"
	ListingResourceType       = "GCP::AnalyticsHub::Listing"
	QueryTemplateResourceType = "GCP::AnalyticsHub::QueryTemplate"
)

var analyticsHubRegistry *base.ResourceRegistry

// nestedParent is shared by listings and query templates: both live under a
// data exchange and address it by the "dataExchange" property.
func nestedParent() *base.ParentResourceConfig {
	return &base.ParentResourceConfig{
		ParentType:     "dataExchanges",
		PropertyName:   "dataExchange",
		RequiresParent: true,
	}
}

func standardOps() []resource.Operation {
	return []resource.Operation{
		resource.OperationCreate,
		resource.OperationRead,
		resource.OperationUpdate,
		resource.OperationDelete,
		resource.OperationList,
		resource.OperationCheckStatus,
	}
}

func init() {
	analyticsHubRegistry = base.NewResourceRegistry(
		AnalyticsHubAPI, AnalyticsHubOperations, AnalyticsHubNativeID)

	err := analyticsHubRegistry.RegisterAll([]base.ResourceDefinition{
		{
			// A data exchange is the container others publish into.
			ResourceType: DataExchangeResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "dataExchanges",
				Scope:              &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam:      "dataExchangeId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			Operations:          standardOps(),
			RequestTransformer:  base.RequestTransformerFunc(dropPathFields),
			ResponseTransformer: shortNameWithLocation("dataExchanges"),
		},
		{
			// A listing publishes one BigQuery dataset into an exchange.
			ResourceType: ListingResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "listings",
				Scope:              &base.ScopeConfig{Type: base.ScopeLocationBased},
				ParentResource:     nestedParent(),
				CreateIDParam:      "listingId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			Operations:          standardOps(),
			RequestTransformer:  base.RequestTransformerFunc(listingRequest),
			ResponseTransformer: base.ResponseTransformerFunc(listingResponse),
		},
		{
			// A query template is a data-clean-room construct: it proposes a
			// routine that subscribers may run against the shared data.
			ResourceType: QueryTemplateResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "queryTemplates",
				Scope:              &base.ScopeConfig{Type: base.ScopeLocationBased},
				ParentResource:     nestedParent(),
				CreateIDParam:      "queryTemplateId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			Operations:          standardOps(),
			RequestTransformer:  base.RequestTransformerFunc(dropPathFields),
			ResponseTransformer: shortNameWithLocation("queryTemplates"),
		},
	})
	if err != nil {
		panic(err)
	}

	// Re-register the two nested types behind a provisioner that walks the
	// exchanges on List. Everything else stays config-driven; only List needs
	// to differ, because neither collection has a URL spanning exchanges.
	for _, resourceType := range []string{ListingResourceType, QueryTemplateResourceType} {
		rt := resourceType
		registry.Register(rt, standardOps(), func(cfg *config.Config) prov.Provisioner {
			def := analyticsHubRegistry.Definitions[rt]
			return &nestedProvisioner{
				BaseResource: &base.BaseResource{
					Config:              cfg,
					APIConfig:           AnalyticsHubAPI,
					OperationConfig:     AnalyticsHubOperations,
					ResourceConfig:      def.ResourceConfig,
					NativeIDConfig:      AnalyticsHubNativeID,
					RequestTransformer:  def.RequestTransformer,
					ResponseTransformer: def.ResponseTransformer,
				},
			}
		})
	}
}
