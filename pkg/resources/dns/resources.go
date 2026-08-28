// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dns

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

const (
	ManagedZoneResourceType        = "GCP::DNS::ManagedZone"
	PolicyResourceType             = "GCP::DNS::Policy"
	ResponsePolicyResourceType     = "GCP::DNS::ResponsePolicy"
	ResponsePolicyRuleResourceType = "GCP::DNS::ResponsePolicyRule"
)

var dnsRegistry *base.ResourceRegistry

func init() {
	dnsRegistry = base.NewResourceRegistry(DNSAPI, DNSOperations, DNSNativeID)

	// ManagedZone fits the generic engine: create is a plain POST to the
	// collection with the name in the body; Read/Delete/List operate on the
	// full resource path. No custom provisioner needed.
	err := dnsRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: ManagedZoneResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "managedZones",
				SupportsUpdate: false, // ponytail: description/labels are patchable; defer until verified
			},
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// A policy governs DNS resolution for the networks attached to it -
			// inbound forwarding, alternative name servers, logging. Plain
			// project-level CRUD.
			ResourceType: PolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "policies",
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPatch,
			},
			RequestTransformer:  base.DropFieldsOnUpdate("name"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			// A response policy holds the rules that override resolution for
			// its networks. Cloud DNS calls its identifier "responsePolicyName"
			// rather than "name", so the transformers translate: a forma
			// declares "name" like every other resource here.
			ResourceType: ResponsePolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "responsePolicies",
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPatch,
			},
			RequestTransformer:  base.RequestTransformerFunc(responsePolicyRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(responsePolicyResponseTransformer),
		},
		{
			// A rule lives under its response policy. Two things the generic
			// engine needs told: the collection is "rules" in the path but
			// "responsePolicyRules" in a list response, and the identifier is
			// "ruleName".
			ResourceType: ResponsePolicyRuleResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "rules",
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "responsePolicies",
					PropertyName:   "responsePolicy",
					RequiresParent: true,
				},
				ListItemsKey:   "responsePolicyRules",
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPatch,
			},
			RequestTransformer:  base.RequestTransformerFunc(responsePolicyRuleRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(responsePolicyRuleResponseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}

	registerResponsePolicyRuleList()
}

// Cloud DNS names the identifier of a response policy "responsePolicyName" and
// that of a rule "ruleName". Every other resource in this plugin declares
// "name", and base builds its path context from that, so the transformers
// translate in both directions rather than leaking the API's inconsistency into
// every forma.

func responsePolicyRequestTransformer(
	props map[string]interface{}, _ base.TransformContext,
) (map[string]interface{}, error) {
	return renameKey(props, "name", "responsePolicyName"), nil
}

func responsePolicyResponseTransformer(
	props map[string]interface{}, _ base.TransformContext,
) map[string]interface{} {
	return renameKey(props, "responsePolicyName", "name")
}

func responsePolicyRuleRequestTransformer(
	props map[string]interface{}, _ base.TransformContext,
) (map[string]interface{}, error) {
	// "responsePolicy" addresses the rule rather than describing it, and the
	// API rejects it as an unknown body field.
	body := renameKey(props, "name", "ruleName")
	delete(body, "responsePolicy")
	return body, nil
}

func responsePolicyRuleResponseTransformer(
	props map[string]interface{}, _ base.TransformContext,
) map[string]interface{} {
	return renameKey(props, "ruleName", "name")
}

// renameKey copies props with one key renamed, leaving the original untouched
// when it is absent.
func renameKey(props map[string]interface{}, from, to string) map[string]interface{} {
	out := make(map[string]interface{}, len(props))
	for k, v := range props {
		if k == from {
			out[to] = v
			continue
		}
		out[k] = v
	}
	return out
}
