// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package pubsub

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
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

// A binding is addressed by resource, role and member together.
func TestIamNativeIDRoundTrip(t *testing.T) {
	id := buildIamNativeID("projects/p/topics/t1", "roles/pubsub.publisher", "serviceAccount:a@b.com")
	res, role, member, err := parseIamNativeID(id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "projects/p/topics/t1" || role != "roles/pubsub.publisher" || member != "serviceAccount:a@b.com" {
		t.Errorf("got %q %q %q", res, role, member)
	}
	if _, _, _, err := parseIamNativeID("projects/p/topics/t1"); err == nil {
		t.Error("an id without role and member must be rejected")
	}
}

func TestResourcePathResolution(t *testing.T) {
	p := &iamMemberProvisioner{collection: "topics"}
	cfg := &config.Config{Project: "proj"}

	got, err := p.resourcePath(map[string]interface{}{"topic": "t1"}, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "projects/proj/topics/t1"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// A full path is kept as-is, so a binding can target another project.
	got, err = p.resourcePath(map[string]interface{}{"topic": "projects/other/topics/t9"}, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "projects/other/topics/t9"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	if _, err := p.resourcePath(map[string]interface{}{}, cfg); err == nil {
		t.Error("a missing target must be rejected")
	}

	sub := &iamMemberProvisioner{collection: "subscriptions"}
	if got := sub.propertyName(); got != "subscription" {
		t.Errorf("propertyName = %q, want subscription", got)
	}
}

// The whole point of modelling a binding rather than a policy: sibling bindings
// must survive, including ones written by principals outside the forma.
func TestAddAndRemoveMemberLeaveSiblingsAlone(t *testing.T) {
	policy := &iamPolicy{Bindings: []iamBinding{
		{Role: "roles/pubsub.viewer", Members: []string{"user:someone@example.com"}},
		{Role: "roles/pubsub.publisher", Members: []string{"serviceAccount:other@x.com"}},
	}}

	if !addMember(policy, "roles/pubsub.publisher", "serviceAccount:me@x.com") {
		t.Fatal("adding a new member must report a change")
	}
	if addMember(policy, "roles/pubsub.publisher", "serviceAccount:me@x.com") {
		t.Error("adding the same member twice must report no change")
	}
	if !hasMember(policy, "roles/pubsub.publisher", "serviceAccount:me@x.com") {
		t.Error("member should be present")
	}

	if !removeMember(policy, "roles/pubsub.publisher", "serviceAccount:me@x.com") {
		t.Fatal("removing a present member must report a change")
	}
	if removeMember(policy, "roles/pubsub.publisher", "serviceAccount:me@x.com") {
		t.Error("removing an absent member must report no change")
	}
	if !hasMember(policy, "roles/pubsub.publisher", "serviceAccount:other@x.com") {
		t.Error("the sibling member in the same role must survive")
	}
	if !hasMember(policy, "roles/pubsub.viewer", "user:someone@example.com") {
		t.Error("the sibling binding in another role must survive")
	}
}

// An empty binding is not a valid policy entry, so the last member out drops it.
func TestRemovingLastMemberDropsBinding(t *testing.T) {
	policy := &iamPolicy{Bindings: []iamBinding{
		{Role: "roles/pubsub.publisher", Members: []string{"serviceAccount:me@x.com"}},
	}}
	removeMember(policy, "roles/pubsub.publisher", "serviceAccount:me@x.com")
	if len(policy.Bindings) != 0 {
		t.Errorf("binding should be gone, got %+v", policy.Bindings)
	}
}

// A conditional binding is a different resource shape; this type must not
// silently adopt or mutate one.
func TestConditionalBindingsAreLeftAlone(t *testing.T) {
	policy := &iamPolicy{Bindings: []iamBinding{
		{
			Role:      "roles/pubsub.publisher",
			Members:   []string{"serviceAccount:me@x.com"},
			Condition: map[string]interface{}{"expression": "true"},
		},
	}}
	if hasMember(policy, "roles/pubsub.publisher", "serviceAccount:me@x.com") {
		t.Error("a conditional binding must not count as a match")
	}
	if removeMember(policy, "roles/pubsub.publisher", "serviceAccount:me@x.com") {
		t.Error("a conditional binding must not be mutated")
	}
	// Adding appends a new unconditional binding rather than touching it.
	if !addMember(policy, "roles/pubsub.publisher", "serviceAccount:me@x.com") {
		t.Error("adding should create an unconditional binding")
	}
	if len(policy.Bindings) != 2 {
		t.Errorf("expected the conditional binding to survive alongside, got %+v", policy.Bindings)
	}
}
