// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package firestore implements GCP Firestore resources.
package firestore

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

const (
	DatabaseResourceType = "GCP::Firestore::Database"
)

var firestoreRegistry *base.ResourceRegistry

func init() {
	firestoreRegistry = base.NewResourceRegistry(
		FirestoreAPI, FirestoreSyncOperations, FirestoreNativeID)

	err := firestoreRegistry.RegisterAll([]base.ResourceDefinition{
		{
			// A Firestore database: the container documents, indexes and fields
			// live in. A project may hold many, so this is a creatable resource
			// rather than the per-project singleton Firestore began as.
			//
			// An empty one costs nothing: Firestore charges for stored bytes
			// and for read/write/delete operations, and a database with no
			// documents has neither. Nothing in the create body provisions
			// capacity. The one paid switch in it is
			// pointInTimeRecoveryEnablement, which the server leaves disabled.
			//
			// Project-scoped: the URL is "/projects/{p}/databases" with no
			// location segment, and where the database lives is the locationId
			// body field.
			ResourceType: DatabaseResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "databases",
				Scope:              &base.ScopeConfig{Type: base.ScopeProjectLevel},
				CreateIDParam:      "databaseId", // id goes in ?databaseId=, not the body
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			// name is the path and locationId and databaseEdition are fixed at
			// creation - the API answers a change to either with "Changing
			// database location is not supported" and "Changing the edition of
			// a database is not supported" - so all three have to leave the
			// body before the mask is built from it.
			//
			// type stays, even though it is the one field with a mask
			// restriction of its own: a PATCH that changes it while masking
			// anything else is refused. Sending it unchanged alongside other
			// fields is accepted, which is what every reconcile does, and
			// keeping it in the body means a type change fails loudly instead
			// of being dropped and silently re-planned forever.
			RequestTransformer:  base.DropFieldsOnUpdate("name", "locationId", "databaseEdition"),
			ResponseTransformer: base.ResponseTransformerFunc(databaseResponseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}
}
