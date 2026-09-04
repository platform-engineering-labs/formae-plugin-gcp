// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package firestore

import (
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// databaseOutputOnlyFields are the fields a Database read carries that no forma
// declares, and that are therefore deleted rather than given a provider
// default.
//
// hasProviderDefault is the right answer when the server's value means
// something a forma might later want to own. None of these do, and three of
// them cannot be tolerated even as provider defaults:
//
//   - "etag" is recomputed on every read. Three consecutive GETs of an
//     untouched database returned three different etags with an identical
//     updateTime. Anything that stores it stores a value that is already stale.
//   - "earliestVersionTime" is "now minus versionRetentionPeriod", so it moves
//     continuously for the same reason.
//   - "realtimeUpdatesMode" is reported on every database and refused on create
//     for most of them: the API answers a Standard-edition create that sets it
//     with "Only Enterprise Edition is allowed to set Realtime Updates Mode",
//     while reporting REALTIME_UPDATES_MODE_ENABLED for that same database
//     afterwards. Declaring it would mean a value that appears in state,
//     survives into an extracted forma, and then fails the create it came
//     from - so it is dropped instead of declared.
//
// The rest are ordinary output-only bookkeeping: identity ("uid"), timestamps
// ("createTime", "updateTime"), the PITR window ("versionRetentionPeriod"), the
// free-tier flag (which the API itself reports as true on a live database and
// false in the delete response for the same one), the Datastore application id
// prefix, the provenance of a restored or cloned database, and the two fields
// that only appear once a database has been deleted.
//
// "enhancedTextSearchQueryMode" is here for a different reason: it is returned
// on every database and appears nowhere in the v1 discovery document, so
// nothing is known about its values or its stability. An undocumented field is
// not a field to declare.
var databaseOutputOnlyFields = []string{
	"uid",
	"createTime",
	"updateTime",
	"versionRetentionPeriod",
	"earliestVersionTime",
	"freeTier",
	"etag",
	"keyPrefix",
	"previousId",
	"deleteTime",
	"sourceInfo",
	"cmekConfig",
	"realtimeUpdatesMode",
	"enhancedTextSearchQueryMode",
}

// databaseResponseTransformer normalises whatever a Firestore call hands back
// into the properties a forma declares.
//
// It has three jobs, and the first exists only because the registry is
// synchronous (see FirestoreSyncOperations). A synchronous Create passes the
// raw POST body to the response transformer, and for this API that body is a
// google.longrunning Operation - "name" is an operation path and the database
// is nested under "response" alongside an "@type" - while Read and the
// post-update read-back both deliver a bare Database. So the envelope is
// unwrapped when present and the body used as-is when not.
//
// Then "name" is shortened from "projects/{p}/databases/{id}" to the id the
// forma declared, and the output-only fields are dropped.
func databaseResponseTransformer(
	apiResponse map[string]interface{},
	ctx base.TransformContext,
) map[string]interface{} {
	body := unwrapOperationResponse(apiResponse)

	if name, ok := body["name"].(string); ok {
		if i := strings.LastIndex(name, "/"); i >= 0 {
			body["name"] = name[i+1:]
		}
	}

	for _, f := range databaseOutputOnlyFields {
		delete(body, f)
	}

	_ = ctx
	return body
}

// unwrapOperationResponse returns the resource carried by a completed
// long-running Operation, or the argument unchanged when it is already a
// resource.
//
// The test is "name is an operation path and response is an object", not merely
// "response is present": a Database has no "response" field of its own, but
// keying off the operation path as well means a future field of that name on
// the resource could not be mistaken for an envelope. "@type" is the Operation
// machinery's type marker and is stripped with the envelope it belongs to.
func unwrapOperationResponse(body map[string]interface{}) map[string]interface{} {
	name, _ := body["name"].(string)
	if !strings.Contains(name, "/operations/") {
		return body
	}
	inner, ok := body["response"].(map[string]interface{})
	if !ok {
		return body
	}
	delete(inner, "@type")
	return inner
}
