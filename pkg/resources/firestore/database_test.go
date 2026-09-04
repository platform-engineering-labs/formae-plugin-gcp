// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package firestore

import (
	"errors"
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// The bodies below are verbatim live responses from
// firestore.googleapis.com/v1 against project development-477117, trimmed only
// of the project id. Keeping them literal is the point: every field dropped
// here is a field the API really sent unbidden, and the round trip is only
// pinned if the input is the shape the API actually produces.

// createResponse is what POST /projects/{p}/databases?databaseId=... answers
// with: a google.longrunning Operation that is already done, carrying the new
// database under "response". A synchronous registry hands this straight to the
// response transformer, so unwrapping it is what stands between the create and
// state that records an operation path as the database's name.
func createResponse() map[string]interface{} {
	return map[string]interface{}{
		"name":     "projects/p/databases/formae-probe-db1/operations/Zqb-g-AF6b9rSbL",
		"metadata": map[string]interface{}{"@type": "type.googleapis.com/google.firestore.admin.v1.CreateDatabaseMetadata"},
		"done":     true,
		"response": map[string]interface{}{
			"@type":                         "type.googleapis.com/google.firestore.admin.v1.Database",
			"name":                          "projects/p/databases/formae-probe-db1",
			"uid":                           "c43192d6-feb2-496b-bfe9-05e083fea666",
			"createTime":                    "2026-09-03T15:07:13.649225Z",
			"updateTime":                    "2026-09-03T15:07:13.649225Z",
			"locationId":                    "europe-central2",
			"type":                          "FIRESTORE_NATIVE",
			"concurrencyMode":               "PESSIMISTIC",
			"versionRetentionPeriod":        "3600s",
			"earliestVersionTime":           "2026-09-03T15:07:13.649225Z",
			"appEngineIntegrationMode":      "DISABLED",
			"pointInTimeRecoveryEnablement": "POINT_IN_TIME_RECOVERY_DISABLED",
			"deleteProtectionState":         "DELETE_PROTECTION_DISABLED",
			"databaseEdition":               "STANDARD",
			"freeTier":                      true,
			"realtimeUpdatesMode":           "REALTIME_UPDATES_MODE_ENABLED",
			"enhancedTextSearchQueryMode":   "ENHANCED_QUERY_MODE_ENABLED",
			"etag":                          "IPCG+43Y0pYDMMnk+o3Y0pYD",
		},
	}
}

// readResponse is what GET /projects/{p}/databases/{id} answers with for the
// same database: a bare Database, no envelope, and a different etag from the
// create's for an untouched resource.
func readResponse() map[string]interface{} {
	return map[string]interface{}{
		"name":                          "projects/p/databases/formae-probe-db1",
		"uid":                           "c43192d6-feb2-496b-bfe9-05e083fea666",
		"createTime":                    "2026-09-03T15:07:13.649225Z",
		"updateTime":                    "2026-09-03T15:07:13.649225Z",
		"locationId":                    "europe-central2",
		"type":                          "FIRESTORE_NATIVE",
		"concurrencyMode":               "PESSIMISTIC",
		"versionRetentionPeriod":        "3600s",
		"earliestVersionTime":           "2026-09-03T15:07:13.649225Z",
		"appEngineIntegrationMode":      "DISABLED",
		"pointInTimeRecoveryEnablement": "POINT_IN_TIME_RECOVERY_DISABLED",
		"deleteProtectionState":         "DELETE_PROTECTION_DISABLED",
		"databaseEdition":               "STANDARD",
		"freeTier":                      true,
		"realtimeUpdatesMode":           "REALTIME_UPDATES_MODE_ENABLED",
		"enhancedTextSearchQueryMode":   "ENHANCED_QUERY_MODE_ENABLED",
		"etag":                          "IIeWn5PY0pYDMIP9+ZDY0pYD",
	}
}

// declaredProperties is the fixture's own declaration, which is what a create
// and every later reconcile send. The transformer's output has to equal this
// exactly, or the very first sync reads drift on a database nobody touched.
func declaredProperties() map[string]interface{} {
	return map[string]interface{}{
		"name":                          "formae-probe-db1",
		"locationId":                    "europe-central2",
		"type":                          "FIRESTORE_NATIVE",
		"concurrencyMode":               "PESSIMISTIC",
		"appEngineIntegrationMode":      "DISABLED",
		"pointInTimeRecoveryEnablement": "POINT_IN_TIME_RECOVERY_DISABLED",
		"deleteProtectionState":         "DELETE_PROTECTION_DISABLED",
		"databaseEdition":               "STANDARD",
	}
}

// Both halves of the round trip land on the declared properties: the create's
// Operation envelope and the read's bare Database. If only one did, declared
// and stored state would disagree on an immutable field and every re-apply
// would plan a replacement that then cannot recreate the id (see the schema's
// note on the five-minute id hold).
func TestDatabaseResponseTransformerRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]interface{}
	}{
		{name: "create: the operation envelope is unwrapped", in: createResponse()},
		{name: "read: a bare database passes through", in: readResponse()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := databaseResponseTransformer(tt.in, base.TransformContext{})
			if !reflect.DeepEqual(got, declaredProperties()) {
				t.Errorf("transform mismatch\n got: %#v\nwant: %#v", got, declaredProperties())
			}
		})
	}
}

// etag is the field that would poison state most quietly: it changes on every
// read of an untouched database, so two reads must still transform to the same
// properties.
func TestDatabaseResponseTransformerIsStableAcrossReads(t *testing.T) {
	first := databaseResponseTransformer(createResponse(), base.TransformContext{})
	second := databaseResponseTransformer(readResponse(), base.TransformContext{})
	if !reflect.DeepEqual(first, second) {
		t.Errorf("two reads of one database transformed differently\n first: %#v\nsecond: %#v", first, second)
	}
}

// A delete answers with an Operation that has no "done" field at all - not
// false, absent - and the finished database under "response". A checker keyed
// only on "done" waits forever on a database that is already gone.
func TestCheckOperationStatus(t *testing.T) {
	tests := []struct {
		name     string
		op       map[string]interface{}
		wantDone bool
		wantErr  bool
	}{
		{
			name:     "create: done is set outright",
			op:       map[string]interface{}{"done": true},
			wantDone: true,
		},
		{
			name: "delete: no done, but a response means finished",
			op: map[string]interface{}{
				"name":     "projects/p/databases/formae-probe-db1/operations/Asm_zMgQBtTmn8wIDBoHEEC",
				"metadata": map[string]interface{}{"@type": "type.googleapis.com/google.firestore.admin.v1.DeleteDatabaseMetadata"},
				"response": map[string]interface{}{"name": "projects/p/databases/c43192d6"},
			},
			wantDone: true,
		},
		{
			name:     "genuinely in progress: neither done nor a response",
			op:       map[string]interface{}{"name": "projects/p/databases/db/operations/x"},
			wantDone: false,
		},
		{
			name:     "a failed operation is done and errored",
			op:       map[string]interface{}{"done": true, "error": map[string]interface{}{"message": "boom"}},
			wantDone: true,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done, err := checkOperationStatus(tt.op)
			if done != tt.wantDone {
				t.Errorf("done = %v, want %v", done, tt.wantDone)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, want error %v", err, tt.wantErr)
			}
		})
	}
}

// The native ID a create records comes from the declared id, because the create
// response's "name" is an operation path. A read or list item, whose "name" is
// the database path already, must give the same answer.
func TestExtractFirestoreNativeID(t *testing.T) {
	const want = "projects/p/databases/formae-probe-db1"

	got := extractFirestoreNativeID(createResponse(), base.PathContext{
		Project:      "p",
		ResourceType: "databases",
		ResourceName: "formae-probe-db1",
	})
	if got != want {
		t.Errorf("create: native ID = %q, want %q", got, want)
	}

	got = extractFirestoreNativeID(readResponse(), base.PathContext{})
	if got != want {
		t.Errorf("read: native ID = %q, want %q", got, want)
	}

	got = extractFirestoreNativeID(createResponse(), base.PathContext{})
	if got != want {
		t.Errorf("envelope without context: native ID = %q, want %q", got, want)
	}
}

// The URL has no location segment: a database's region is a body field, so a
// target's configured region must not reach the path.
func TestFirestorePathBuilder(t *testing.T) {
	collection := firestorePathBuilder(base.PathContext{
		Project:      "p",
		Region:       "europe-central2",
		ResourceType: "databases",
	})
	if collection != "/projects/p/databases" {
		t.Errorf("collection path = %q", collection)
	}

	single := firestorePathBuilder(base.PathContext{
		Project:      "p",
		Region:       "europe-central2",
		ResourceType: "databases",
		ResourceName: "formae-probe-db1",
	})
	if single != "/projects/p/databases/formae-probe-db1" {
		t.Errorf("resource path = %q", single)
	}
}

// Firestore answers a patch issued while a create is still settling with
// 409 ABORTED and a request to try again. It has to be classified retryable, or
// a reconcile - which patches seconds after creating - fails on a database that
// is perfectly healthy.
func TestIsConcurrentDatabaseChange(t *testing.T) {
	cases := map[string]bool{
		`googleapi: Error 409: There are concurrent database changes, please try again., aborted`: true,
		`rpc error: ABORTED: something happened, please try again`:                                true,
		`googleapi: Error 404: Operation does not exist`:                                          false,
		`googleapi: Error 400: Invalid argument`:                                                  false,
		``:                                                                                        false,
	}
	for msg, want := range cases {
		var err error
		if msg != "" {
			err = errors.New(msg)
		}
		if got := isConcurrentDatabaseChange(err); got != want {
			t.Errorf("isConcurrentDatabaseChange(%q) = %v, want %v", msg, got, want)
		}
	}
	if isConcurrentDatabaseChange(nil) {
		t.Error("nil error must not be retryable")
	}
}
