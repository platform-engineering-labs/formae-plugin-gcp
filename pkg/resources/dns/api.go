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

// dnsPathBuilder builds /projects/{project}[/{parentType}/{parent}]/{resourceType}[/{name}].
// A response policy rule is the one nested resource here:
// /projects/{p}/responsePolicies/{policy}/rules/{rule}.
func dnsPathBuilder(ctx base.PathContext) string {
	path := "/projects/" + ctx.Project
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
func extractDNSNativeID(response map[string]interface{}, ctx base.PathContext) string {
	name := ctx.ResourceName
	if name == "" {
		// Cloud DNS does not agree with itself about what the identifier is
		// called: a policy and a managed zone use "name", a response policy uses
		// "responsePolicyName" and a rule uses "ruleName".
		for _, key := range []string{"name", "responsePolicyName", "ruleName"} {
			if n, ok := response[key].(string); ok && n != "" {
				name = n
				break
			}
		}
	}
	if name == "" {
		return ""
	}
	if ctx.ParentType != "" && ctx.ParentResource != "" {
		// A record set is identified by name *and* type; the type is a path
		// segment of its own rather than part of the name.
		if ctx.ResourceType == "rrsets" && !strings.Contains(name, "/") {
			if recordType, ok := response["type"].(string); ok && recordType != "" {
				name += "/" + recordType
			}
		}
		return fmt.Sprintf("projects/%s/%s/%s/%s/%s",
			ctx.Project, ctx.ParentType, ctx.ParentResource, ctx.ResourceType, name)
	}
	return fmt.Sprintf("projects/%s/%s/%s", ctx.Project, ctx.ResourceType, name)
}

// parseDNSNativeID parses "projects/{project}/{resourceType}/{name}" and the one
// nested shape, "projects/{p}/responsePolicies/{policy}/rules/{rule}". A nested
// resource has to restore its parent, or a read would address the project-level
// collection and 404.
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
	case 7:
		// A record set is the one resource here addressed by two path segments:
		// .../rrsets/{name}/{type}. Both are carried in ResourceName joined by a
		// slash, which the path builder appends verbatim - a DNS name may
		// contain dots but never a slash, so the join is unambiguous.
		if parts[2] != "managedZones" || parts[4] != "rrsets" {
			return base.PathContext{}, fmt.Errorf("invalid DNS native ID: %s", nativeID)
		}
		return base.PathContext{
			Project:        parts[1],
			ParentType:     parts[2],
			ParentResource: parts[3],
			ResourceType:   parts[4],
			ResourceName:   parts[5] + "/" + parts[6],
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
		return base.PathContext{}, fmt.Errorf("invalid DNS native ID: %s", nativeID)
	}
}
