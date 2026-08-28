// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
)

// Known gaps that predate this test. Each is a real defect, not an exemption on
// principle: a schema without a provisioner is declarable but fails at apply,
// and a provisioner without a schema cannot be declared at all. The test locks
// in today's state so new drift fails loudly; shrink this list, never grow it.
var knownParityGaps = map[string]string{
	"GCP::Bigtable::Backup":             "schema/pkl/bigtable/backup.pkl has no provisioner",
	"GCP::Storage::ObjectAccessControl": "registration is commented out in pkg/resources/storage/resources.go (object-scoped ACLs need both bucket and object in the path)",
}

var schemaTypeRE = regexp.MustCompile(`const type = "(GCP::[^"]+)"`)

// schemaTypes collects the resource types the published PKL schema declares.
func schemaTypes(t *testing.T) map[string]string {
	t.Helper()
	found := map[string]string{}
	err := filepath.Walk("schema/pkl", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".pkl") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if m := schemaTypeRE.FindSubmatch(body); m != nil {
			found[string(m[1])] = path
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking schema/pkl: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("no schema types found - has schema/pkl moved?")
	}
	return found
}

// Every declarable resource type must have a provisioner behind it. Without
// this, a schema whose Go registration silently fails to land still passes
// verify-schema and only breaks when a user applies it.
func TestEverySchemaTypeHasAProvisioner(t *testing.T) {
	registered := map[string]bool{}
	for _, name := range registry.RegisteredTypes() {
		registered[name] = true
	}

	var missing []string
	for typ, path := range schemaTypes(t) {
		if registered[typ] {
			continue
		}
		if _, known := knownParityGaps[typ]; known {
			continue
		}
		missing = append(missing, typ+" ("+path+")")
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("schema types with no provisioner - a forma declaring these fails at apply:\n  %s",
			strings.Join(missing, "\n  "))
	}
}

// And the reverse: a provisioner nobody can declare is dead weight, and usually
// means the schema module was forgotten.
func TestEveryProvisionerHasASchemaType(t *testing.T) {
	declared := schemaTypes(t)

	var orphaned []string
	for _, typ := range registry.RegisteredTypes() {
		if _, ok := declared[typ]; ok {
			continue
		}
		if _, known := knownParityGaps[typ]; known {
			continue
		}
		// The conformance harness registers a synthetic type for its own checks.
		if typ == "GCP::Svc::Resource" {
			continue
		}
		orphaned = append(orphaned, typ)
	}
	sort.Strings(orphaned)
	if len(orphaned) > 0 {
		t.Errorf("provisioners with no schema module - these cannot be declared in a forma:\n  %s",
			strings.Join(orphaned, "\n  "))
	}
}

// The known-gap list must not rot: once a gap is fixed, its entry has to go, or
// the list quietly starts hiding real regressions.
func TestKnownParityGapsAreStillGaps(t *testing.T) {
	registered := map[string]bool{}
	for _, name := range registry.RegisteredTypes() {
		registered[name] = true
	}
	declared := schemaTypes(t)

	for typ, why := range knownParityGaps {
		_, hasSchema := declared[typ]
		if hasSchema && registered[typ] {
			t.Errorf("%s is no longer a gap (%s) - remove it from knownParityGaps", typ, why)
		}
	}
}
