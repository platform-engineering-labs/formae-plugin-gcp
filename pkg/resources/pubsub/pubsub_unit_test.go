// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package pubsub

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// A snapshot's create body is CreateSnapshotRequest, not the Snapshot resource:
// it carries the subscription (expanded to a full path) and nothing the API
// only reports back.
func TestSnapshotCreateTransformer(t *testing.T) {
	body, err := snapshotCreateTransformer(map[string]interface{}{
		"name":         "snap-1",
		"subscription": "sub-1",
		"topic":        "topic-1",
		"expireTime":   "2026-01-01T00:00:00Z",
		"labels":       map[string]interface{}{"env": "test"},
	}, base.TransformContext{Project: "proj"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := body["subscription"], "projects/proj/subscriptions/sub-1"; got != want {
		t.Errorf("subscription = %v, want %v", got, want)
	}
	for _, k := range []string{"name", "topic", "expireTime"} {
		if _, ok := body[k]; ok {
			t.Errorf("%q must not be sent on create", k)
		}
	}
	if _, ok := body["labels"]; !ok {
		t.Error("labels must be sent on create")
	}
}

// An already-qualified subscription path is passed through untouched.
func TestSnapshotCreateTransformerKeepsFullPath(t *testing.T) {
	body, err := snapshotCreateTransformer(map[string]interface{}{
		"subscription": "projects/other/subscriptions/sub-1",
	}, base.TransformContext{Project: "proj"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := body["subscription"], "projects/other/subscriptions/sub-1"; got != want {
		t.Errorf("subscription = %v, want %v", got, want)
	}
}

// Read returns full paths; state holds short names.
func TestSnapshotResponseTransformer(t *testing.T) {
	out := snapshotResponseTransformer(map[string]interface{}{
		"name":  "projects/proj/snapshots/snap-1",
		"topic": "projects/proj/topics/topic-1",
	}, base.TransformContext{})

	if got, want := out["name"], "snap-1"; got != want {
		t.Errorf("name = %v, want %v", got, want)
	}
	if got, want := out["topic"], "topic-1"; got != want {
		t.Errorf("topic = %v, want %v", got, want)
	}
}
