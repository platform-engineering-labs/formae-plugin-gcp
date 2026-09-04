// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package apikeys

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// keyServerOwnedFields are the top-level fields the API sets for itself.
//
// None is declared in the schema, and a read that carried them would present
// Verify with fields the forma has no opinion about. `etag` is worth a word:
// it is recomputed on every write and this API takes it as a delete query
// parameter, never as payload, so replaying it would be wrong as well as
// noisy — the create response and the very next GET already disagree about it.
//
// `deleteTime` is deliberately absent from this list: it is the tombstone
// marker, and keyDeleted below has to see it. See ReadTreatAsMissing in
// resources.go.
var keyServerOwnedFields = []string{"uid", "etag", "createTime", "updateTime"}

// keyResponseTransformer drops the key material, drops the server-owned
// bookkeeping, and shortens the full-path name to the declared id.
//
// The keyString drop is the point of this function. An API key IS a
// credential — the string is what a caller puts in an `?key=` query parameter —
// and keys.create returns it inline in the completed operation's payload. It
// must not reach stored state as plaintext, so it is removed here, on every
// path, rather than being relied upon to be absent.
//
// It is absent from most paths: keys.get, keys.list and keys.patch all omit it
// (verified live, and the discovery document says so of patch), and it is
// reachable only through the separate keys.getKeyString method, which nothing
// in this plugin calls. The create response is the one exception, and it is the
// exception that matters, because create is where a fresh secret exists.
//
// This is the same treatment compute vpnTunnels.sharedSecret gets, and for the
// same reason. It is NOT the secretVersion.data treatment: that payload is an
// input the author wrote and marked writeOnly, whereas keyString is generated
// by the server and has no declared counterpart at all — so there is nothing
// to declare writeOnly, only something to refuse to store.
func keyResponseTransformer(
	apiResponse map[string]interface{}, ctx base.TransformContext,
) map[string]interface{} {
	delete(apiResponse, "keyString")
	for _, f := range keyServerOwnedFields {
		delete(apiResponse, f)
	}
	return base.ShortNameResponseTransformer.Transform(apiResponse, ctx)
}

// keyDeleted reports whether a read has come back with a tombstone.
//
// keys.delete answers with an Operation that completes successfully, and the
// key then keeps reading back HTTP 200 for THIRTY DAYS with `deleteTime` set,
// waiting for a possible keys.undelete (verified live: a GET immediately after
// a completed delete returns the key with deleteTime populated). Only the list
// call hides it, and only by default — ?showDeleted=true brings it back.
//
// Without this hook every synchronization inside that month reads a deleted key
// as a live one and restores it to inventory. That is a longer window than any
// other tombstone in this plugin by four orders of magnitude — Memorystore's
// aclPolicies linger for fifteen seconds — so unlike that case there is no
// chance of a later sync happening to fall outside it.
func keyDeleted(body map[string]interface{}) bool {
	deleteTime, _ := body["deleteTime"].(string)
	return deleteTime != ""
}
