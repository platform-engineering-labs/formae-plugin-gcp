// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package kms

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// KMSAPI - Cloud KMS API v1. KeyRings are location-scoped. create returns the
// KeyRing directly (synchronous); get/list are plain reads. KeyRings have no
// delete or patch method in the API — they are permanent once created.
var KMSAPI = base.APIConfig{
	BaseURL:     "https://cloudkms.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: kmsPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize", PageTokenParam: "pageToken"},
}

// KMSOperations - Cloud KMS admin operations are synchronous. create returns
// the resource directly, so there is no operation to poll.
var KMSOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractKMSNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// KMSNativeID - full path
// "projects/{project}/locations/{location}/keyRings/{name}".
var KMSNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
}

// kmsPathBuilder builds
// /projects/{project}/locations/{location}/{resourceType}[/{name}].
func kmsPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s/%s", ctx.Project, ctx.Location, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractKMSNativeID returns the full resource path. On a sync create/get the
// response carries it in "name" as
// "projects/{p}/locations/{l}/keyRings/{name}"; otherwise build it from the
// declared id in context.
func extractKMSNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name, ok := response["name"].(string); ok && strings.HasPrefix(name, "projects/") {
		return name
	}
	if ctx.ResourceName != "" {
		return fmt.Sprintf("projects/%s/locations/%s/%s/%s",
			ctx.Project, ctx.Location, ctx.ResourceType, ctx.ResourceName)
	}
	return ""
}
