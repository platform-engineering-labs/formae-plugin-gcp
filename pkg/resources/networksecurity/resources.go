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

	ClientTlsPolicyResourceType             = "GCP::NetworkSecurity::ClientTlsPolicy"
	ServerTlsPolicyResourceType             = "GCP::NetworkSecurity::ServerTlsPolicy"
	BackendAuthenticationConfigResourceType = "GCP::NetworkSecurity::BackendAuthenticationConfig"
	AuthorizationPolicyResourceType         = "GCP::NetworkSecurity::AuthorizationPolicy"
	GatewaySecurityPolicyResourceType       = "GCP::NetworkSecurity::GatewaySecurityPolicy"
	GatewaySecurityPolicyRuleResourceType   = "GCP::NetworkSecurity::GatewaySecurityPolicyRule"
	DnsThreatDetectorResourceType           = "GCP::NetworkSecurity::DnsThreatDetector"
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
		{
			// The client half of a TLS connection Google's proxies make on your
			// behalf: which certificate to present to an upstream, which CAs to
			// trust in its answer, and the SNI to send. A policy on its own
			// connects to nothing - a backend service or a mesh route names it -
			// and it is free.
			ResourceType: ClientTlsPolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "clientTlsPolicies",
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "clientTlsPolicyId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			// name is the path, and the mask is built from the body, so it has
			// to leave before a patch. description and sni are both patchable.
			RequestTransformer:  base.DropFieldsOnUpdate("name"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// The server half: which certificate a Google-managed proxy serves,
			// and whether it demands a client certificate in return. allowOpen
			// is the "serve plaintext as well" switch, and is what makes a
			// policy declarable without a certificate to point at.
			ResourceType: ServerTlsPolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "serverTlsPolicies",
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "serverTlsPolicyId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  base.DropFieldsOnUpdate("name"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// What a load balancer trusts when it opens a TLS connection to a
			// backend: the public root set, or a named trust config, plus an
			// optional client certificate to present.
			ResourceType: BackendAuthenticationConfigResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "backendAuthenticationConfigs",
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "backendAuthenticationConfigId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			// etag is server-owned and, unlike most of this API, actually
			// returned: replaying it in a patch body would both enter the
			// update mask and stake a claim on a version the server has already
			// moved past.
			RequestTransformer:  base.DropFieldsOnUpdate("name", "etag"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// Who may talk to a service mesh workload: a list of match rules and
			// one verdict, ALLOW or DENY, for whatever matches. Enforced by the
			// Envoy sidecars a mesh route attaches it to; the policy itself
			// costs nothing and enforces nothing on its own.
			ResourceType: AuthorizationPolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "authorizationPolicies",
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "authorizationPolicyId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			// The rules are echoed back exactly as sent, nested objects and
			// all, so nothing but the name needs translating.
			RequestTransformer:  base.DropFieldsOnUpdate("name"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// The container for Secure Web Proxy rules, and the thing a
			// gateway points at. Regional: it is the second collection in this
			// API that locations/global rejects outright, this time as
			// "Malformed name", so it carries no Scope and is absent from
			// globalResourceTypes - see api.go.
			//
			// A policy that still has rules refuses to delete with HTTP 400, so
			// the rules must go first. A rule references its policy, which makes
			// the policy the producer on a default edge, and formae destroys the
			// consumer first - so the declared reference is also what gets the
			// order right.
			ResourceType: GatewaySecurityPolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "gatewaySecurityPolicies",
				CreateIDParam:      "gatewaySecurityPolicyId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  base.DropFieldsOnUpdate("name"),
			ResponseTransformer: gatewaySecurityPolicyResponseTransformer,
		},
		{
			// One rule of a Secure Web Proxy policy: a session matcher, a
			// priority, and ALLOW or DENY. Nested under its policy, which is a
			// path component rather than a body field.
			//
			// ListItemsKey is set because the collection segment ("rules") is
			// not the key the list response uses; base tries "items" and the
			// resource type before falling back to it, so naming the response
			// key here is harmless either way and is the difference between a
			// discoverable rule and an invisible one.
			ResourceType: GatewaySecurityPolicyRuleResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "rules",
				CreateIDParam: "gatewaySecurityPolicyRuleId",
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "gatewaySecurityPolicies",
					PropertyName:   "gatewaySecurityPolicy",
					RequiresParent: true,
				},
				ListItemsKey:       "gatewaySecurityPolicyRules",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer:  gatewaySecurityPolicyRuleRequestTransformer,
			ResponseTransformer: gatewaySecurityPolicyRuleResponseTransformer,
		},
		{
			// A subscription that has Cloud DNS logs answered by a threat
			// intelligence provider, so a lookup of a known-malicious domain is
			// flagged. The detector is the plumbing; the provider is named by
			// "provider" and holds the intelligence.
			//
			// The one synchronous collection in this API: POST and PATCH return
			// the resource, not an Operation, which is why it carries its own
			// OperationConfig instead of the registry's. See
			// NetworkSecuritySyncOperations for why the async path cannot be
			// left to degrade.
			ResourceType:    DnsThreatDetectorResourceType,
			OperationConfig: NetworkSecuritySyncOperations,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "dnsThreatDetectors",
				Scope:              &base.ScopeConfig{Type: base.ScopeGlobal},
				CreateIDParam:      "dnsThreatDetectorId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			// provider is fixed at creation - swapping intelligence providers is
			// a different subscription - so it leaves the body rather than
			// entering the update mask, alongside the name that is the path.
			RequestTransformer:  base.DropFieldsOnUpdate("name", "provider"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
	})
	if err != nil {
		panic(err)
	}
}
