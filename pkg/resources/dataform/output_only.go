// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dataform

import (
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// internalMetadata is on every one of Dataform's four resources, and is the
// reason each of them needs a response transformer at all.
//
// It is a JSON string the service keeps its own bookkeeping in - insert and
// update timestamps, a CCFE id, a state - and it changes on every write:
//
//	"internalMetadata": "{\"db_metadata_insert_time\":\"...\",
//	  \"db_metadata_update_time\":\"...\",\"state\":\"ACTIVE\",
//	  \"unique_ccfe_id\":\"...\"}"
//
// Nothing can declare it and nothing can converge on it, so it is dropped
// rather than carried as a provider default: a hasProviderDefault field is
// still a field a forma may write, and this one would then differ from the
// stored value the moment anything patched the resource.
const internalMetadata = "internalMetadata"

// dropFields returns a ResponseTransformer that removes the named top-level
// fields from a read-back.
//
// Every one of these is a field GCP populates and no forma declares, so leaving
// it in place reads as perpetual drift on a resource nobody touched. The
// alternative - declaring it with hasProviderDefault - is right when the
// provider's value means something a forma might later want to own; none of
// Dataform's do.
func dropFields(fields ...string) base.ResponseTransformer {
	return base.ResponseTransformerFunc(
		func(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
			for _, f := range fields {
				delete(apiResponse, f)
			}
			return apiResponse
		})
}

// parentRepositoryOf lifts the owning repository out of a nested resource's
// full path,
// projects/{p}/locations/{loc}/repositories/{repo}/{collection}/{name}.
//
// It returns "" for anything else - a repository's own path, or a name already
// shortened - so a response that is not shaped like a nested resource leaves
// the property alone rather than gaining a wrong one.
func parentRepositoryOf(name string) string {
	parts := strings.Split(name, "/")
	if len(parts) == 8 && parts[0] == "projects" && parts[2] == "locations" && parts[4] == "repositories" {
		return parts[5]
	}
	return ""
}

// restoreRepository puts the owning repository back into a nested resource's
// read-back.
//
// The repository is a path component, never a body field - the API answers a
// create carrying one with 400 "Unknown name \"repository\" at 'workspace':
// Cannot find field" - so it is dropped from every request, and this is the
// only thing that can put it back. Without it the property a forma declared is
// simply missing from state and reads as drift.
//
// It is recovered from the reported name rather than from the transform
// context, which is what makes discovery work: a List goes through the "-"
// wildcard repository, so the context has no parent to offer, but every listed
// item still reports its own full path.
//
// Must run before ShortNameResponseTransformer, which throws the path away.
var restoreRepository = base.ResponseTransformerFunc(
	func(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
		if name, ok := apiResponse["name"].(string); ok {
			if repo := parentRepositoryOf(name); repo != "" {
				apiResponse["repository"] = repo
			}
		}
		return apiResponse
	})
