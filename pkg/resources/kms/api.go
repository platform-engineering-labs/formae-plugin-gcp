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
// KeyRing directly (synchronous); get/list are plain reads. keyRings.delete
// exists and is effectively synchronous - see KMSOperations. There is no
// keyRings.patch, so a change to the one declarable field replaces.
var KMSAPI = base.APIConfig{
	BaseURL:     "https://cloudkms.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: kmsPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize", PageTokenParam: "pageToken"},
}

// KMSOperations - Cloud KMS admin operations are synchronous. create returns
// the resource directly, so there is no operation to poll.
//
// delete is the one that looks otherwise: it answers with an Operation. That
// Operation is already done when it arrives - it carries no "done" field and no
// metadata beyond the type - and the ring 404s on the next GET. Treating it as
// synchronous is therefore correct, and necessary: the Operation's name is an
// operations path, so polling it as if it were the resource would address the
// wrong thing, and NativeIDExtractor would mistake it for the ring's own name.
// BaseResource.Delete never reads the body on the synchronous path, so it does
// not.
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
