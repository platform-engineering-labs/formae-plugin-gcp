// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"reflect"
	"testing"
)

func metadataItems(pairs ...string) []interface{} {
	items := make([]interface{}, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		items = append(items, map[string]interface{}{"key": pairs[i], "value": pairs[i+1]})
	}
	return items
}

func TestMetadataItemNativeIDRoundTrip(t *testing.T) {
	const path = "projects/dev-1/commonInstanceMetadata/items/startup-script"
	if got := buildMetadataItemNativeID("dev-1", "startup-script"); got != path {
		t.Fatalf("build: %q", got)
	}
	project, key, err := parseMetadataItemNativeID(path)
	if err != nil {
		t.Fatal(err)
	}
	if project != "dev-1" || key != "startup-script" {
		t.Errorf("parse: %q %q", project, key)
	}
	for _, bad := range []string{
		"projects/dev-1/commonInstanceMetadata/items/", // no key
		"projects/dev-1/commonInstanceMetadata/items",  // the collection
		"projects/dev-1/global/metadata/items/k",       // wrong shape
		"",
	} {
		if _, _, err := parseMetadataItemNativeID(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

// setCommonInstanceMetadata replaces the whole list, so anything this function
// drops is dropped from the project. Foreign keys must always survive.
func TestMergeMetadataItemPreservesForeignKeys(t *testing.T) {
	existing := metadataItems("enable-oslogin", "true", "ssh-keys", "alice:ssh-rsa AAA")

	added := mergeMetadataItem(existing, "startup-script", "#!/bin/sh", false)
	want := []map[string]interface{}{
		{"key": "enable-oslogin", "value": "true"},
		{"key": "ssh-keys", "value": "alice:ssh-rsa AAA"},
		{"key": "startup-script", "value": "#!/bin/sh"},
	}
	if !reflect.DeepEqual(added, want) {
		t.Errorf("add: %#v", added)
	}

	// Overwriting keeps the key in place rather than moving it to the end.
	replaced := mergeMetadataItem(existing, "enable-oslogin", "false", false)
	if replaced[0]["key"] != "enable-oslogin" || replaced[0]["value"] != "false" {
		t.Errorf("overwrite in place: %#v", replaced)
	}
	if len(replaced) != 2 || replaced[1]["key"] != "ssh-keys" {
		t.Errorf("overwrite lost a key: %#v", replaced)
	}

	// Removing takes out exactly one key.
	removed := mergeMetadataItem(existing, "ssh-keys", "", true)
	if len(removed) != 1 || removed[0]["key"] != "enable-oslogin" {
		t.Errorf("remove: %#v", removed)
	}

	// Removing a key that was never there leaves everything alone.
	untouched := mergeMetadataItem(existing, "not-present", "", true)
	if len(untouched) != 2 {
		t.Errorf("removing an absent key changed the list: %#v", untouched)
	}
}

// A project with no metadata at all, and junk entries, must not panic or wipe.
func TestMergeMetadataItemEdgeCases(t *testing.T) {
	if got := mergeMetadataItem(nil, "k", "v", false); len(got) != 1 || got[0]["key"] != "k" {
		t.Errorf("empty project: %#v", got)
	}
	if got := mergeMetadataItem(nil, "k", "", true); len(got) != 0 {
		t.Errorf("remove from empty: %#v", got)
	}
	junk := []interface{}{
		"not-a-map",
		map[string]interface{}{"value": "keyless"},
		map[string]interface{}{"key": "real", "value": "kept"},
		map[string]interface{}{"key": "novalue"},
	}
	got := mergeMetadataItem(junk, "added", "v", false)
	keys := []string{}
	for _, item := range got {
		keys = append(keys, item["key"].(string))
	}
	if !reflect.DeepEqual(keys, []string{"real", "novalue", "added"}) {
		t.Errorf("junk handling: %#v", keys)
	}
	// A key with no value must not gain a bogus empty one that would look like drift.
	for _, item := range got {
		if item["key"] == "novalue" {
			if _, ok := item["value"]; ok {
				t.Errorf("valueless key gained a value: %#v", item)
			}
		}
	}
}

func TestMetadataItemValue(t *testing.T) {
	items := metadataItems("a", "1", "b", "2")
	if v, ok := metadataItemValue(items, "b"); !ok || v != "2" {
		t.Errorf("lookup: %q %v", v, ok)
	}
	if _, ok := metadataItemValue(items, "missing"); ok {
		t.Error("absent key reported present")
	}
	if _, ok := metadataItemValue(nil, "a"); ok {
		t.Error("empty list reported a key")
	}
}
