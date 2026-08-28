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
