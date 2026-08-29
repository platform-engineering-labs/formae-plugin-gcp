// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package base

import "testing"

// A listed item identified by something other than "name" must still yield a
// native ID. Cloud Storage ACL entries are identified by "entity" and carry no
// name; requiring one made them undiscoverable, and the list came back empty
// with no error - indistinguishable from "none exist".
func TestExtractNativeIDFromItemUsesTheExtractorFirst(t *testing.T) {
	b := &BaseResource{
		OperationConfig: OperationConfig{
			NativeIDExtractor: func(item map[string]interface{}, ctx PathContext) string {
				if entity, ok := item["entity"].(string); ok && entity != "" {
					return "b/" + ctx.ParentResource + "/acl/" + entity
				}
				return ""
			},
		},
	}

	got := b.extractNativeIDFromItem(
		map[string]interface{}{"entity": "project-editors-1"},
		PathContext{ParentResource: "my-bucket"},
	)
	if want := "b/my-bucket/acl/project-editors-1"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// With no extractor and no name there is nothing to build from.
func TestExtractNativeIDFromItemNeedsSomething(t *testing.T) {
	b := &BaseResource{}
	if got := b.extractNativeIDFromItem(map[string]interface{}{"entity": "x"}, PathContext{}); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// A name-shaped item still works when the extractor declines.
func TestExtractNativeIDFromItemFallsBackToName(t *testing.T) {
	b := &BaseResource{
		NativeIDConfig: NativeIDConfig{Format: SimpleNameFormat},
		OperationConfig: OperationConfig{
			NativeIDExtractor: func(map[string]interface{}, PathContext) string { return "" },
		},
	}
	if got := b.extractNativeIDFromItem(map[string]interface{}{"name": "thing"}, PathContext{}); got != "thing" {
		t.Errorf("a named item should still produce a native ID, got %q", got)
	}
}
