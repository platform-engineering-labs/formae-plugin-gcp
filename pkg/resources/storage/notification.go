// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package storage

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// A bucket notification names its topic in a form used nowhere else in GCP:
//
//	//pubsub.googleapis.com/projects/{project}/topics/{topic}
//
// A forma passes a resolvable that resolves to a bare topic name - so formae
// creates the topic first - which the request expands and the response shortens
// back. Without both halves the declared value could never equal the value read
// back, and every comparison step would report drift on a notification that is
// in fact correct.
const notificationTopicPrefix = "//pubsub.googleapis.com/"

// notificationRequest drops the bucket, which addresses the notification in the
// URL, and expands the topic.
func notificationRequest(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		switch k {
		case "bucket", "id", "selfLink":
			// bucket is a path component; id and selfLink are server-assigned.
			continue
		}
		body[k] = v
	}

	topic, ok := body["topic"].(string)
	if !ok || topic == "" {
		return body, nil
	}
	if strings.HasPrefix(topic, notificationTopicPrefix) {
		return body, nil
	}
	if !strings.HasPrefix(topic, "projects/") {
		topic = fmt.Sprintf("projects/%s/topics/%s", ctx.Project, topic)
	}
	body["topic"] = notificationTopicPrefix + topic
	return body, nil
}

// notificationResponse shortens the topic and recovers the bucket.
//
// The bucket is not a field of the notification - it lives only in the URL, and
// a read would otherwise drop a value the forma declared. TransformContext does
// not carry the parent, but selfLink does:
//
//	https://www.googleapis.com/storage/v1/b/{bucket}/notificationConfigs/{id}
func notificationResponse(props map[string]interface{}, _ base.TransformContext) map[string]interface{} {
	out := make(map[string]interface{}, len(props)+1)
	for k, v := range props {
		out[k] = v
	}

	if topic, ok := out["topic"].(string); ok {
		topic = strings.TrimPrefix(topic, notificationTopicPrefix)
		if i := strings.LastIndex(topic, "/topics/"); i >= 0 {
			topic = topic[i+len("/topics/"):]
		}
		out["topic"] = topic
	}

	if bucket := bucketFromSelfLink(props["selfLink"]); bucket != "" {
		out["bucket"] = bucket
	}
	return out
}

// bucketFromSelfLink pulls the bucket out of a notification's selfLink.
func bucketFromSelfLink(raw interface{}) string {
	link, ok := raw.(string)
	if !ok {
		return ""
	}
	const marker = "/b/"
	i := strings.Index(link, marker)
	if i < 0 {
		return ""
	}
	rest := link[i+len(marker):]
	if j := strings.Index(rest, "/"); j >= 0 {
		return rest[:j]
	}
	return rest
}
