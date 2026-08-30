// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dns

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// DNSAPI configuration for the Cloud DNS API v1.
var DNSAPI = base.APIConfig{
	BaseURL:     "https://dns.googleapis.com/dns/v1",
	APIVersion:  "v1",
	PathBuilder: dnsPathBuilder,
}

// DNSOperations - Cloud DNS resource operations are synchronous.
var DNSOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractDNSNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// DNSNativeID - full resource path "projects/{project}/managedZones/{name}".
var DNSNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseDNSNativeID,
}

// dnsPathBuilder builds /projects/{project}/{resourceType}[/{name}], and for a
// resource that hangs off another - a response policy rule under its response
// policy - /projects/{project}/{parentType}/{parent}/{resourceType}[/{name}].
func dnsPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s", ctx.Project)
	if ctx.ParentType != "" && ctx.ParentResource != "" {
		path += fmt.Sprintf("/%s/%s", ctx.ParentType, ctx.ParentResource)
	}
	path += "/" + ctx.ResourceType
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractDNSNativeID builds the full resource path. Cloud DNS responses carry
// the short name in "name"; the path is project + resourceType + name.
//
// A response policy is the exception: its id field is "responsePolicyName". A
// listed item carries no path context to fall back on, so without this every
// response policy would list with an empty native ID and never be discovered.
func extractDNSNativeID(response map[string]interface{}, ctx base.PathContext) string {
	name := ctx.ResourceName
	if name == "" {
		if n, ok := response["name"].(string); ok {
			name = n
		}
	}
	if name == "" {
		if n, ok := response["responsePolicyName"].(string); ok {
			name = n
		}
	}
	if name == "" {
		if n, ok := response["ruleName"].(string); ok {
			name = n
		}
	}
	if name == "" {
		return ""
	}
	if ctx.ParentType != "" && ctx.ParentResource != "" {
		return fmt.Sprintf("projects/%s/%s/%s/%s/%s",
			ctx.Project, ctx.ParentType, ctx.ParentResource, ctx.ResourceType, name)
	}
	return fmt.Sprintf("projects/%s/%s/%s", ctx.Project, ctx.ResourceType, name)
}

// parseDNSNativeID parses "projects/{project}/{resourceType}/{name}" and the
// nested "projects/{project}/{parentType}/{parent}/{resourceType}/{name}".
func parseDNSNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if parts[0] != "projects" {
		return base.PathContext{}, fmt.Errorf("invalid DNS native ID: %s", nativeID)
	}
	switch len(parts) {
	case 4:
		return base.PathContext{
			Project:      parts[1],
			ResourceType: parts[2],
			ResourceName: parts[3],
		}, nil
	case 6:
		return base.PathContext{
			Project:        parts[1],
			ParentType:     parts[2],
			ParentResource: parts[3],
			ResourceType:   parts[4],
			ResourceName:   parts[5],
		}, nil
	default:
		return base.PathContext{}, fmt.Errorf("invalid DNS native ID: %s (expected 4 or 6 segments)", nativeID)
	}
}
