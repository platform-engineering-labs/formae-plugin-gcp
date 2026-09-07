// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Conformance fixtures must name every resource they create
// "formae-plugin-sdk-test-<abbrev>-<testRunID>" (or the underscore variant where
// the API demands one). The cleanup sweeps in scripts/ci key off that shape; a
// fixture that names something else creates a resource no sweep ever collects,
// and those leak until someone notices the bill. This test is the guard.
//
// The exceptions below are resources whose GCP id caps too short to hold the
// 23-character prefix plus the 8-character run ID. They keep the older
// "formae-test-" shape, which SWEEP_RE also matches. Adding to this list means
// you checked the cap; do not add to it to silence a failure.
var shortPrefixExceptions = map[string]string{
	// IAM service account accountId caps at 30.
	"iam-service-account.pkl":         "formae-test-sa-",
	"iam-service-account-replace.pkl": "formae-test-sa-r2-",
	// Spanner database id caps at 30.
	"spanner-database.pkl":               "formae-test-sp-db-",
	"spanner-database-replace.pkl":       "formae-test-sp-db2-",
	"spanner-backup-schedule.pkl":        "formae-test-sp-bs-",
	"spanner-backup-schedule-update.pkl": "formae-test-sp-bs-",
	// Bigtable instance id caps at 33 (Bigtable quotas, "ID length limits").
	"bigtable-instance.pkl":           "formae-test-instance-",
	"bigtable-cluster.pkl":            "formae-test-instance-cl-",
	"bigtable-table.pkl":              "formae-test-instance-tbl-",
	"bigtable-backup.pkl":             "formae-test-instance-bk-",
	"bigtable-materialized-view.pkl":  "formae-test-instance-mv-",
	"bigtable-app-profile.pkl":        "formae-test-btap-",
	"bigtable-app-profile-update.pkl": "formae-test-btap-",
	// VPC Access connector names cap below 21-25 - far under the long prefix.
	"vpcaccess-connector.pkl": "formae-test-conn-",
}

// Any string literal that interpolates the test run ID names a live resource.
var runIDLiteral = regexp.MustCompile(`"([^"\\]*)\\\(v\.testRunID\)`)

// The three prefixes a fixture may use, longest first. No trailing separator:
// the segment after the prefix is joined with "-", "_" or "/" depending on what
// the API accepts. Kept in step with FIXTURE_PREFIX_RE in
// scripts/ci/sweep-patterns.sh, which is what actually sweeps them.
var allowedPrefixes = []string{
	"formae-plugin-sdk-test", "formae_plugin_sdk_test",
	"formae-plugin-test", "formae_plugin_test",
	"formae-test", "formae_test",
}

func hasAllowedPrefix(name string) bool {
	for _, p := range allowedPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func TestFixtureNamesCarryTheSweepPrefix(t *testing.T) {
	files, err := filepath.Glob("testdata/*.pkl")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no fixtures found - is the glob still right?")
	}

	for _, path := range files {
		base := filepath.Base(path)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range runIDLiteral.FindAllStringSubmatch(string(src), -1) {
			name := m[1]
			// A name is sometimes carried under a path or a hostname label,
			// e.g. "custom.googleapis.com/formae_..." for a metric descriptor
			// or "www.formae-...-rrset-<id>.example.com." for a record set.
			name = strings.TrimRight(name, "/.")
			if i := strings.LastIndex(name, "/"); i >= 0 {
				name = name[i+1:]
			}
			if i := strings.LastIndex(name, "."); i >= 0 {
				name = name[i+1:]
			}
			if hasAllowedPrefix(name) {
				continue
			}
			t.Errorf("%s: name %q\\(v.testRunID) must start with one of %v; "+
				"anything else is invisible to the cleanup sweeps in scripts/ci",
				base, m[1], allowedPrefixes)
		}
	}
}

// Each fixture listed in shortPrefixExceptions must actually use the short
// prefix recorded for it. The map is the human-readable record of which GCP id
// caps too short for the default prefix; if a fixture moves off it the record is
// stale and the cap note beside the name is misleading.
func TestShortPrefixExceptionsAreStillNeeded(t *testing.T) {
	for base, prefix := range shortPrefixExceptions {
		src, err := os.ReadFile(filepath.Join("testdata", base))
		if err != nil {
			t.Errorf("%s: listed as a short-prefix exception but unreadable: %v", base, err)
			continue
		}
		if !strings.Contains(string(src), `"`+prefix) {
			t.Errorf("%s: listed as a short-prefix exception using %q, but no name uses it - "+
				"drop the entry, or fix the name", base, prefix)
		}
	}
}

// A per-case sweep names only the segment after FIXTURE_PREFIX_RE, e.g.
//
//	security-policy-rule)  PREFIX_RE="${FIXTURE_PREFIX_RE}spr-"
//
// If that segment and the fixture's names ever disagree, the sweep matches
// nothing and reports success, which looks exactly like a clean project. That is
// how this repo leaked SSL certificates, secrets, and per-node-hour Bigtable
// instances. Pin every segment to the fixture whose case name it is filed under.
var sweepCase = regexp.MustCompile(
	`(?m)^\s*([a-z0-9-]+)\)\s+PREFIX_RE="\$\{FIXTURE_PREFIX_RE\}([^"]*)"`)

func TestSweepSegmentsMatchTheirFixtures(t *testing.T) {
	scripts, err := filepath.Glob("scripts/ci/clean-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, script := range scripts {
		src, err := os.ReadFile(script)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range sweepCase.FindAllStringSubmatch(string(src), -1) {
			caseName, segment := m[1], m[2]
			if caseName == "all" || segment == "" {
				continue // the whole-environment sweep owns no single fixture
			}
			fixture := filepath.Join("testdata", caseName+".pkl")
			body, err := os.ReadFile(fixture)
			if err != nil {
				t.Errorf("%s: case %q sweeps %q but %s does not exist - "+
					"a case that cannot run cannot leak, so drop the entry",
					filepath.Base(script), caseName, segment, fixture)
				continue
			}
			checked++
			if !anyPrefixWith(string(body), segment) {
				t.Errorf("%s: case %q sweeps %q, but no name in %s starts with any allowed "+
					"prefix followed by it - this sweep collects nothing",
					filepath.Base(script), caseName, segment, fixture)
			}
		}
	}
	t.Logf("checked %d sweep segments", checked)
	if checked == 0 {
		t.Fatal("no sweep segments checked - has the PREFIX_RE convention changed?")
	}
}

// anyPrefixWith reports whether src names something as <allowed prefix><segment>.
// A segment may be an ERE alternation, e.g. "(src|dst|stream)-"; every branch
// must be present, since the sweep relies on all of them.
func anyPrefixWith(src, segment string) bool {
	for _, alt := range expandAlternation(segment) {
		found := false
		for _, p := range allowedPrefixes {
			// Prefixes are recorded without their trailing separator.
			for _, sep := range []string{"-", "_"} {
				if strings.Contains(src, `"`+p+sep+alt) {
					found = true
				}
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func expandAlternation(segment string) []string {
	open := strings.Index(segment, "(")
	close := strings.Index(segment, ")")
	if open < 0 || close < open {
		return []string{segment}
	}
	var out []string
	for _, branch := range strings.Split(segment[open+1:close], "|") {
		out = append(out, segment[:open]+branch+segment[close+1:])
	}
	return out
}
