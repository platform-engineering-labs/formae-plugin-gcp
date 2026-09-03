//go:build unit

// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package main

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
)

// The README's table is the only list of supported types anyone outside this
// repository reads, and it had drifted 52 types behind the plugin - a third of
// what shipped was undocumented. Nothing noticed, because nothing was looking.
//
// This is deliberately not a "docs should be tidy" test. A batch lands roughly
// weekly; without a check the table is stale again within one, and the next
// person to ask "does this plugin support X?" gets the wrong answer.
func TestReadmeListsEverySupportedType(t *testing.T) {
	body, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}

	documented := map[string]bool{}
	for _, m := range regexp.MustCompile("(?m)^\\| `(GCP::[A-Za-z]+::[A-Za-z]+)`").FindAllStringSubmatch(string(body), -1) {
		documented[m[1]] = true
	}

	registered := map[string]bool{}
	for _, rt := range registry.RegisteredTypes() {
		registered[rt] = true
	}

	var undocumented, stale []string
	for rt := range registered {
		if !documented[rt] {
			undocumented = append(undocumented, rt)
		}
	}
	for rt := range documented {
		if !registered[rt] {
			stale = append(stale, rt)
		}
	}
	sort.Strings(undocumented)
	sort.Strings(stale)

	if len(undocumented) > 0 {
		t.Errorf("%d type(s) ship but are not in the README table - add a row for each:\n  %v",
			len(undocumented), undocumented)
	}
	if len(stale) > 0 {
		t.Errorf("%d type(s) are in the README table but no longer ship - remove them:\n  %v",
			len(stale), stale)
	}
}

// The count in the prose above the table is what a reader sees first, so it has
// to agree with the table under it.
func TestReadmeCountMatchesTheTable(t *testing.T) {
	body, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	rows := len(regexp.MustCompile("(?m)^\\| `GCP::[A-Za-z]+::[A-Za-z]+`").FindAllString(string(body), -1))
	m := regexp.MustCompile(`This plugin supports \*\*(\d+) GCP resource types\*\*`).FindStringSubmatch(string(body))
	if m == nil {
		t.Fatal("README no longer states how many resource types are supported")
	}
	if m[1] != "" && rows != atoiOrZero(m[1]) {
		t.Errorf("README says %s types but the table has %d rows", m[1], rows)
	}
}

func atoiOrZero(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
