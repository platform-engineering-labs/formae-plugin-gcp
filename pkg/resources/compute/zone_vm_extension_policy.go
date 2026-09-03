// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// vmExtensionPolicyEchoedMembers are the members of an `extensionPolicies` entry
// that GCP adds to a stored policy on its own and reports back as an empty
// string. They are dropped on read when - and only when - they are empty.
//
// `pinnedVersion` is deliberately NOT in this list even though "" is a legal and
// common value for it: "" is what a forma says to mean "track the current
// release", so it is a *declared* empty string and stripping it would invent the
// very drift this transformer exists to remove.
var vmExtensionPolicyEchoedMembers = []string{"stringConfig"}

// zoneVmExtensionPolicyResponseTransformer normalizes a ZoneVmExtensionPolicy
// read.
//
// After an update the API echoes each `extensionPolicies` entry with a
// `stringConfig: ""` it was never sent (observed live on the global sibling,
// which shares this resource body). The entry is a mapping value, so a
// hasProviderDefault hint on `extensionPolicies` cannot reach into it, and the
// added empty member makes every read after the first update disagree with the
// declaration - a patch planned forever that changes nothing. So an empty
// `stringConfig` comes off here.
//
// Top-level provider noise (kind, id, selfLink, selfLinkWithId,
// creationTimestamp, updateTimestamp) is left alone, as everywhere else in this
// package: none is a schema field, so none is compared, and id and selfLink are
// what `res.id` / `res.selfLink` resolve against - removing them would break
// every reference to the policy. `state`, `managedByGlobal` and
// `globalResourceLink` are schema fields carrying hasProviderDefault rather than
// being stripped, so a forma that declares one still gets drift detection.
func zoneVmExtensionPolicyResponseTransformer(
	apiResponse map[string]interface{}, _ base.TransformContext,
) map[string]interface{} {
	result := make(map[string]interface{}, len(apiResponse))
	for k, v := range apiResponse {
		result[k] = v
	}

	policies, ok := result["extensionPolicies"].(map[string]interface{})
	if !ok {
		return result
	}

	cleaned := make(map[string]interface{}, len(policies))
	for name, raw := range policies {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			cleaned[name] = raw
			continue
		}
		copied := make(map[string]interface{}, len(entry))
		for k, v := range entry {
			copied[k] = v
		}
		for _, member := range vmExtensionPolicyEchoedMembers {
			if s, isString := copied[member].(string); isString && s == "" {
				delete(copied, member)
			}
		}
		cleaned[name] = copied
	}
	result["extensionPolicies"] = cleaned

	return result
}

// ZoneVmExtensionPolicyResponseTransformer is the response transformer for
// ZoneVmExtensionPolicy.
var ZoneVmExtensionPolicyResponseTransformer = base.ResponseTransformerFunc(
	zoneVmExtensionPolicyResponseTransformer,
)
