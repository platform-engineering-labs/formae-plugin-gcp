// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import "testing"

// buildMetadata must read the pairs from the schema's nested `items` map and
// emit the GCE API's `items: [{key,value}]` array. Regression: it previously
// iterated the outer object, saw only the non-string "items" value, and
// produced an empty list — silently dropping startup-script and all metadata.
func TestBuildMetadataNestedItems(t *testing.T) {
	got := buildMetadata(map[string]interface{}{
		"items": map[string]interface{}{
			"startup-script": "#!/bin/bash\necho hi",
			"foo":            "bar",
		},
	})
	items, ok := got["items"].([]map[string]interface{})
	if !ok || len(items) != 2 {
		t.Fatalf("expected 2 items, got %#v", got["items"])
	}
	seen := map[string]string{}
	for _, it := range items {
		seen[it["key"].(string)] = it["value"].(string)
	}
	if seen["startup-script"] != "#!/bin/bash\necho hi" || seen["foo"] != "bar" {
		t.Fatalf("wrong pairs: %#v", seen)
	}
}

// Flat shape (no "items" wrapper) must still work — the fallback path.
func TestBuildMetadataFlatFallback(t *testing.T) {
	got := buildMetadata(map[string]interface{}{"k": "v"})
	items := got["items"].([]map[string]interface{})
	if len(items) != 1 || items[0]["key"] != "k" || items[0]["value"] != "v" {
		t.Fatalf("flat fallback broken: %#v", items)
	}
}
