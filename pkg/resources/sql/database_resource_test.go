// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package sql

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// A GCP::SQL::Database is nested under its instance, so its path and native ID
// carry the parent instance: /projects/{p}/instances/{i}/databases/{name}.
func TestNestedDatabasePathAndNativeID(t *testing.T) {
	ctx := base.PathContext{
		Project:        "p",
		ParentType:     "instances",
		ParentResource: "inst",
		ResourceType:   "databases",
		ResourceName:   "formae",
	}
	if got, want := sqlPathBuilder(ctx), "/projects/p/instances/inst/databases/formae"; got != want {
		t.Errorf("nested path = %q, want %q", got, want)
	}

	// Collection path (create POST) omits the database name.
	coll := ctx
	coll.ResourceName = ""
	if got, want := sqlPathBuilder(coll), "/projects/p/instances/inst/databases"; got != want {
		t.Errorf("collection path = %q, want %q", got, want)
	}

	// Native ID round-trips through the parser.
	nid := "projects/p/instances/inst/databases/formae"
	parsed, err := parseSQLNativeID(nid)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.ParentType != "instances" || parsed.ParentResource != "inst" ||
		parsed.ResourceType != "databases" || parsed.ResourceName != "formae" || parsed.Project != "p" {
		t.Errorf("parsed = %+v", parsed)
	}
}

// The top-level instance native ID must still parse (4 segments).
func TestInstanceNativeIDStillParses(t *testing.T) {
	parsed, err := parseSQLNativeID("projects/p/instances/inst")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.ResourceType != "instances" || parsed.ResourceName != "inst" || parsed.ParentType != "" {
		t.Errorf("parsed = %+v", parsed)
	}
}

// The database resource type must be registered in the SQL registry.
func TestDatabaseResourceRegistered(t *testing.T) {
	if _, ok := sqlRegistry.Definitions[DatabaseResourceType]; !ok {
		t.Fatalf("%s not registered", DatabaseResourceType)
	}
}
