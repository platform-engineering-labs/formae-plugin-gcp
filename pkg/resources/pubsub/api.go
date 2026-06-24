// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package pubsub

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// PubSubAPI configuration for the Cloud Pub/Sub API v1.
var PubSubAPI = base.APIConfig{
	BaseURL:     "https://pubsub.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: pubsubPathBuilder,
}

// PubSubOperations - Pub/Sub admin operations are synchronous (no LRO).
var PubSubOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractPubSubNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// PubSubNativeID - native IDs are the full resource path returned by the API,
// e.g. "projects/{project}/topics/{topic}".
var PubSubNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parsePubSubNativeID,
}

// pubsubPathBuilder builds Pub/Sub paths:
//   - collection: /projects/{project}/{resourceType}
//   - resource:   /projects/{project}/{resourceType}/{name}
func pubsubPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/%s", ctx.Project, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractPubSubNativeID returns the full resource path. Pub/Sub responses carry
// it in the "name" field already as "projects/{p}/{type}/{name}".
func extractPubSubNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name, ok := response["name"].(string); ok && strings.HasPrefix(name, "projects/") {
		return name
	}
	if ctx.ResourceName != "" {
		return fmt.Sprintf("projects/%s/%s/%s", ctx.Project, ctx.ResourceType, ctx.ResourceName)
	}
	return ""
}

// parsePubSubNativeID parses "projects/{project}/{resourceType}/{name}".
func parsePubSubNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 4 || parts[0] != "projects" {
		return base.PathContext{}, fmt.Errorf("invalid pubsub native ID: %s", nativeID)
	}
	return base.PathContext{
		Project:      parts[1],
		ResourceType: parts[2],
		ResourceName: parts[3],
	}, nil
}
