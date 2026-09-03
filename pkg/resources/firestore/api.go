// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package firestore

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// FirestoreAPI - Firestore Admin API v1.
//
// Databases hang directly off the project - "/projects/{p}/databases" - with no
// location segment: where a database lives is the "locationId" body field, not
// a path component, so a target's region never reaches the URL.
var FirestoreAPI = base.APIConfig{
	BaseURL:     "https://firestore.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: firestorePathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// FirestoreSyncOperations - synchronous, and not as a shortcut.
//
// Every mutating Firestore Admin call answers with a google.longrunning
// Operation, so the async path looks like the obvious fit. It is not: two of the
// three operations cannot be polled at all, and both failures were measured
// against the live API rather than inferred.
//
//   - PATCH names an operation that does not exist. The response carries
//     "projects/{p}/databases/{db}/operations/{id}", and a GET of that exact
//     path answers HTTP 404 "Operation does not exist" - immediately, and still
//     minutes later, while the patch itself has plainly been applied.
//     projects.databases.operations.list is empty too. base.Status treats a 404
//     as a definitive answer rather than a transient poll error, so every
//     update would be reported as a failure on a database it had just
//     successfully changed.
//   - DELETE names an operation that never finishes. The GET succeeds and
//     returns the deleted database under "response", but the envelope has no
//     "done" field at all - not false, absent - so a poller waits for a flag
//     that is never set while the database is already gone.
//
// Only create polls correctly, and it does not need to: the POST answers with
// "done": true and the finished database inline, in about a second, and a GET
// issued straight afterwards returns it. So there is nothing for the
// asynchronous path to buy and two ways for it to break.
//
// Synchronous changes the shape of what each call hands back, and
// databaseResponseTransformer is what absorbs that: Create passes the raw POST
// body - the Operation envelope, not a Database - straight to the response
// transformer, while Read and the post-update read-back deliver a bare
// Database. The transformer unwraps the one and passes the other through.
var FirestoreSyncOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractFirestoreNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// FirestoreNativeID - "projects/{project}/databases/{database}".
//
// No custom parser: base's default path parser walks key/value pairs, so
// "projects" sets the project and the unrecognised "databases" pair falls
// through to ResourceType and ResourceName, which is exactly right here.
var FirestoreNativeID = base.NativeIDConfig{
	Format:       base.FullPathFormat,
	PathTemplate: "projects/{project}/databases/{name}",
}

// firestorePathBuilder builds /projects/{project}/databases[/{name}].
func firestorePathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/%s", ctx.Project, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractOperationName returns the Operation name from a mutating response.
// Retained for completeness of the OperationConfig; with Synchronous set base
// never polls, and see FirestoreSyncOperations for why it must not.
func extractOperationName(response map[string]interface{}) string {
	if name, ok := response["name"].(string); ok && strings.Contains(name, "/operations/") {
		return name
	}
	return ""
}

// extractFirestoreNativeID builds "projects/{p}/databases/{name}".
//
// A create answers with an Operation whose "name" is an operation path, so the
// response cannot supply the database's own path; buildPathContext has already
// put the declared id in ctx.ResourceName, which is what gets used. The
// fallbacks cover a read or a list item, where "name" is the full database path
// already, and the Operation envelope's embedded response.
func extractFirestoreNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if ctx.ResourceName != "" {
		return fmt.Sprintf("projects/%s/%s/%s", ctx.Project, ctx.ResourceType, ctx.ResourceName)
	}
	if inner, ok := response["response"].(map[string]interface{}); ok {
		if name, ok := inner["name"].(string); ok {
			if i := strings.Index(name, "projects/"); i >= 0 {
				return name[i:]
			}
		}
	}
	if name, ok := response["name"].(string); ok && !strings.Contains(name, "/operations/") {
		if i := strings.Index(name, "projects/"); i >= 0 {
			return name[i:]
		}
	}
	return ""
}

// checkOperationStatus reports whether a polled Operation is done.
//
// "done" alone is not enough for this API: a delete's operation carries the
// finished resource under "response" and never sets "done". AIP-151 puts
// "response" and "error" in an Operation only once it has completed, so their
// presence is a sound completion signal in its own right, and treating it as
// one is what keeps this function honest if the registry is ever switched off
// Synchronous.
func checkOperationStatus(op map[string]interface{}) (bool, error) {
	if errObj, ok := op["error"].(map[string]interface{}); ok {
		msg, _ := errObj["message"].(string)
		if msg == "" {
			msg = "operation failed"
		}
		return true, fmt.Errorf("%s", msg)
	}
	if done, _ := op["done"].(bool); done {
		return true, nil
	}
	_, hasResponse := op["response"]
	return hasResponse, nil
}
