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

// dnsPathBuilder builds /projects/{project}/{resourceType}[/{name}].
func dnsPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/%s", ctx.Project, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractDNSNativeID builds the full resource path. Cloud DNS responses carry
// the short name in "name"; the path is project + resourceType + name.
func extractDNSNativeID(response map[string]interface{}, ctx base.PathContext) string {
	name := ctx.ResourceName
	if name == "" {
		if n, ok := response["name"].(string); ok {
			name = n
		}
	}
	if name == "" {
		return ""
	}
	return fmt.Sprintf("projects/%s/%s/%s", ctx.Project, ctx.ResourceType, name)
}

// parseDNSNativeID parses "projects/{project}/{resourceType}/{name}".
func parseDNSNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 4 || parts[0] != "projects" {
		return base.PathContext{}, fmt.Errorf("invalid DNS native ID: %s", nativeID)
	}
	return base.PathContext{
		Project:      parts[1],
		ResourceType: parts[2],
		ResourceName: parts[3],
	}, nil
}
