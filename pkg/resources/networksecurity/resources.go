// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package networksecurity implements GCP Network Security resources.
package networksecurity

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

const (
	AddressGroupResourceType         = "GCP::NetworkSecurity::AddressGroup"
	UrlListResourceType              = "GCP::NetworkSecurity::UrlList"
	SecurityProfileResourceType      = "GCP::NetworkSecurity::SecurityProfile"
	SecurityProfileGroupResourceType = "GCP::NetworkSecurity::SecurityProfileGroup"
)

var networkSecurityRegistry *base.ResourceRegistry

func init() {
	networkSecurityRegistry = base.NewResourceRegistry(
		NetworkSecurityAPI, NetworkSecurityOperations, NetworkSecurityNativeID)

	err := networkSecurityRegistry.RegisterAll([]base.ResourceDefinition{
		{
			// A named, reusable set of IP addresses and CIDR blocks that
			// firewall policies and Cloud Armor rules match against, so a rule
			// refers to one group rather than restating every address.
			ResourceType: AddressGroupResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "addressGroups",
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "addressGroupId", // id goes in ?addressGroupId=, not the body
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			// name is the path. type and capacity are fixed at creation - the
			// API answers a capacity change with "capacity can't be changed" -
			// and the mask is built from the body, so both have to leave it.
			RequestTransformer:  base.DropFieldsOnUpdate("name", "type", "capacity"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// A named list of URL patterns for Secure Web Proxy policies to
			// match on.
			//
			// Regional, alone in this API. No ScopeGlobal here: that clears
			// ctx.Location, and locations/global is not merely empty for this
			// collection, it is rejected as an invalid location.
			ResourceType: UrlListResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "urlLists",
				CreateIDParam:      "urlListId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  base.DropFieldsOnUpdate("name"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// The policy half of Cloud NGFW's layer-7 inspection: what to do
			// about a threat, not where to apply it. Creating one provisions
			// nothing - it becomes billable only once a firewall endpoint,
			// which is an organization-level resource, is attached to it.
			ResourceType: SecurityProfileResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "securityProfiles",
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "securityProfileId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  base.RequestTransformerFunc(securityProfileRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(securityProfileResponseTransformer),
		},
		{
			// The binding a firewall policy rule actually names: a group that
			// gathers up to one security profile of each kind.
			ResourceType: SecurityProfileGroupResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "securityProfileGroups",
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "securityProfileGroupId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			// The profile references are full paths on the wire but short names
			// in a forma; see security_profile_group.go for why both halves of
			// that translation have to exist.
			RequestTransformer:  base.RequestTransformerFunc(securityProfileGroupRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(securityProfileGroupResponseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}
}
