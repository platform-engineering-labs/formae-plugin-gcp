// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package redis

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// RedisSyncOperations is the operation config for the aclPolicies collection.
//
// Everything else in this API is a long-running operation, but an ACL policy is
// not: POST .../aclPolicies?aclPolicyId=x answers HTTP 200 with the finished
// resource ("state": "ACTIVE"), and DELETE answers 200 with an empty body. The
// API's own discovery document says so - the `state` enum documents "Since ACL
// policy creation is synchronous and not an LRO, there is no CREATING state" -
// and it has no CREATING member.
//
// base supports a per-resource override: ResourceRegistry.Register replaces a
// definition's OperationConfig with the registry-wide one ONLY when the
// definition's OperationIDExtractor is nil. So an override must set that field
// even where it is never called, which is why extractOperationName is wired in
// below: without it this whole struct would be silently discarded and the
// registry's async config used instead.
//
// For the record, what the async path would have done here (this is why the
// override matters, not merely why it is tidier):
//   - Create: extractOperationName only matches a name containing
//     "/operations/", and this response's name is the resource path, so the
//     operation ID is "". BaseResource.Create does not special-case that on the
//     create path: it reports OperationStatusInProgress with an empty RequestID
//     and no properties, and the subsequent Status poll would GET
//     "https://redis.googleapis.com/v1/" - the base URL with an empty operation
//     path - which is not the created policy and does not report on it.
//   - Delete: BaseResource.Delete does special-case an empty operation ID and
//     reports success, so delete alone would have degraded correctly.
//
// Only delete degrades; create does not. Hence Synchronous: true.
var RedisSyncOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      base.LocationNativeIDExtractor,
	OperationStatusChecker: checkOperationStatus,
}

// aclPolicyDeleting reports whether a read has come back with a tombstone.
//
// aclPolicies.delete answers HTTP 200 and the policy then reads back for
// another fifteen to twenty seconds with "state": "DELETING" - measured against
// the live API - before the GET finally 404s. A synchronization landing inside
// that window would otherwise read a deleted policy as a live one and restore
// it to inventory, where it would sit until a later sync happened to fall
// outside the window. Wired in as ResourceConfig.ReadTreatAsMissing, which
// turns such a read into NotFound.
//
// Only DELETING counts. CREATING does not exist in this API - the state enum
// documents that creation is synchronous and has no such member - so there is
// no in-progress state to confuse with a tombstone.
func aclPolicyDeleting(body map[string]interface{}) bool {
	state, _ := body["state"].(string)
	return state == "DELETING"
}

// aclPolicyOutputOnly are the fields the API owns and will not accept in a
// request body. They are declared in the schema (so a read-back that carries
// them is not an unexpected field) but must never be sent: PATCH builds its
// updateMask from the body's top-level keys, so any of them left in place turns
// a rules change into a rejected call. `etag` in particular is a delete query
// parameter in this API, never payload.
var aclPolicyOutputOnly = []string{"state", "version", "etag", "createTime", "updateTime"}

// aclPolicyRequestTransformer strips the server-owned fields from every request
// body, and strips `name` from update bodies only.
//
// `name` has to survive a create: base pulls the id out of body["name"] to send
// it as ?aclPolicyId= and removes it itself. On update it is the path, and
// UpdateMaskFromBody would otherwise put it in the mask - the API rejects a
// masked `name` because a policy cannot be renamed.
func aclPolicyRequestTransformer(
	props map[string]interface{}, ctx base.TransformContext,
) (map[string]interface{}, error) {
	drop := make(map[string]bool, len(aclPolicyOutputOnly)+1)
	for _, f := range aclPolicyOutputOnly {
		drop[f] = true
	}
	drop["clusterAclPolicyAttachments"] = true
	if ctx.Operation == resource.OperationUpdate {
		drop["name"] = true
	}

	out := make(map[string]interface{}, len(props))
	for k, v := range props {
		if drop[k] {
			continue
		}
		out[k] = v
	}
	return out, nil
}

// aclPolicyResponseTransformer shortens the full resource path the API reports
// in `name` back to the short id the forma declared, and strips
// `clusterAclPolicyAttachments`.
//
// The strip is not cosmetic. That field is an output-only array of per-cluster
// attachment objects, and a schema `hasProviderDefault` hint reaches only
// top-level fields: Verify compares a nested structure as a whole, so an
// attachment appearing once some cluster references this policy would read as
// drift against a forma that never mentioned it. The top-level scalars
// (`state`, `version`, `etag`, `createTime`, `updateTime`) are waved through by
// hints instead, so they stay in state where they are useful.
func aclPolicyResponseTransformer(
	apiResponse map[string]interface{}, ctx base.TransformContext,
) map[string]interface{} {
	out := base.ShortNameResponseTransformer.Transform(apiResponse, ctx)
	delete(out, "clusterAclPolicyAttachments")
	return out
}
