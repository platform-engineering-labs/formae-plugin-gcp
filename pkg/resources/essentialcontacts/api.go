// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package essentialcontacts

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// EssentialContactsAPI configuration for the Essential Contacts API v1.
var EssentialContactsAPI = base.APIConfig{
	BaseURL:     "https://essentialcontacts.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: essentialContactsPathBuilder,
}

// EssentialContactsOperations - Essential Contacts operations are synchronous:
// contacts.create returns the Contact directly and delete returns Empty (no LRO).
var EssentialContactsOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractEssentialContactsNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// EssentialContactsNativeID - full resource path
// "projects/{project}/contacts/{contactId}". The contact id is server-assigned.
var EssentialContactsNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseEssentialContactsNativeID,
}

// essentialContactsPathBuilder builds project-scoped paths:
//   - collection: /projects/{project}/contacts
//   - resource:   /projects/{project}/contacts/{contactId}
func essentialContactsPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/%s", ctx.Project, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractEssentialContactsNativeID returns the full resource path. The contact
// name is server-assigned, so the create response carries it in "name" already
// as "projects/{project}/contacts/{contactId}".
func extractEssentialContactsNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name, ok := response["name"].(string); ok && strings.HasPrefix(name, "projects/") {
		return name
	}
	if ctx.ResourceName != "" {
		return fmt.Sprintf("projects/%s/%s/%s", ctx.Project, ctx.ResourceType, ctx.ResourceName)
	}
	return ""
}

// parseEssentialContactsNativeID parses
// "projects/{project}/contacts/{contactId}".
func parseEssentialContactsNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 4 || parts[0] != "projects" {
		return base.PathContext{}, fmt.Errorf("invalid essential contacts native ID: %s", nativeID)
	}
	return base.PathContext{
		Project:      parts[1],
		ResourceType: parts[2],
		ResourceName: parts[3],
	}, nil
}
