// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package storage

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// The topic form a bucket notification wants is used nowhere else in GCP.
func TestNotificationTopicRoundTrip(t *testing.T) {
	body, err := notificationRequest(map[string]interface{}{
		"bucket":         "b1",
		"topic":          "t1",
		"payload_format": "JSON_API_V1",
		"id":             "7",
		"selfLink":       "https://example/ignored",
	}, base.TransformContext{Project: "proj"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := body["topic"], "//pubsub.googleapis.com/projects/proj/topics/t1"; got != want {
		t.Errorf("topic = %v, want %v", got, want)
	}
	for _, k := range []string{"bucket", "id", "selfLink"} {
		if _, ok := body[k]; ok {
			t.Errorf("%q must not be sent on create", k)
		}
	}

	out := notificationResponse(map[string]interface{}{
		"topic":    "//pubsub.googleapis.com/projects/proj/topics/t1",
		"id":       "7",
		"selfLink": "https://www.googleapis.com/storage/v1/b/b1/notificationConfigs/7",
	}, base.TransformContext{})
	if got, want := out["topic"], "t1"; got != want {
		t.Errorf("topic = %v, want %v", got, want)
	}
	// bucket is not a field of the notification; it lives only in the URL.
	if got, want := out["bucket"], "b1"; got != want {
		t.Errorf("bucket = %v, want %v", got, want)
	}
}

// An already-qualified topic must not be prefixed twice.
func TestNotificationTopicKeepsQualifiedForm(t *testing.T) {
	full := "//pubsub.googleapis.com/projects/other/topics/t9"
	body, err := notificationRequest(map[string]interface{}{"topic": full}, base.TransformContext{Project: "proj"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := body["topic"]; got != full {
		t.Errorf("topic = %v, want %v", got, full)
	}

	// A bare projects/... path gets the prefix but not a second projects/ segment.
	body, err = notificationRequest(map[string]interface{}{
		"topic": "projects/other/topics/t9",
	}, base.TransformContext{Project: "proj"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := body["topic"]; got != full {
		t.Errorf("topic = %v, want %v", got, full)
	}
}

func TestBucketFromSelfLink(t *testing.T) {
	cases := map[string]string{
		"https://www.googleapis.com/storage/v1/b/my-bucket/notificationConfigs/3": "my-bucket",
		"https://www.googleapis.com/storage/v1/b/my-bucket":                       "my-bucket",
		"nonsense": "",
	}
	for link, want := range cases {
		if got := bucketFromSelfLink(link); got != want {
			t.Errorf("%q -> %q, want %q", link, got, want)
		}
	}
	if got := bucketFromSelfLink(nil); got != "" {
		t.Errorf("nil -> %q, want empty", got)
	}
}

// An ACL's native ID is b/{bucket}/{collection}/{entity}, which
// parseStorageNativeID must round-trip - the walking List builds these by hand.
func TestAclNativeIDRoundTrip(t *testing.T) {
	for _, collection := range []string{"acl", "defaultObjectAcl"} {
		id := "b/my-bucket/" + collection + "/project-editors-123"
		ctx, err := parseStorageNativeID(id)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", collection, err)
		}
		if ctx.ParentResource != "my-bucket" {
			t.Errorf("%s: bucket = %q", collection, ctx.ParentResource)
		}
		if ctx.ResourceType != collection {
			t.Errorf("%s: resourceType = %q", collection, ctx.ResourceType)
		}
		if ctx.ResourceName != "project-editors-123" {
			t.Errorf("%s: entity = %q", collection, ctx.ResourceName)
		}
	}
}

// An object name may contain slashes - Cloud Storage has no directories, only
// names that look like paths - so everything after "/o/" is the name.
func TestObjectNativeIDRoundTrip(t *testing.T) {
	for _, name := range []string{"file.txt", "a/b/c.json", "deep/nested/path/x"} {
		id := objectNativeID("my-bucket", name)
		bucket, got, err := parseObjectNativeID(id)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", name, err)
		}
		if bucket != "my-bucket" || got != name {
			t.Errorf("%s -> bucket %q name %q", id, bucket, got)
		}
	}
	for _, bad := range []string{"my-bucket", "b/my-bucket", "b//o/x", "b/my-bucket/o/"} {
		if _, _, err := parseObjectNativeID(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

// The upload endpoint differs from the JSON one only in the segment before
// /storage/v1, and getting it wrong answers 404 rather than naming the problem.
func TestUploadBase(t *testing.T) {
	got := uploadBase("https://storage.googleapis.com/storage/v1")
	if want := "https://storage.googleapis.com/upload/storage/v1"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
