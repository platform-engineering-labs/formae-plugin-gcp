// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package orgpolicy

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// OrgPolicyAPI configuration for the Organization Policy API v2.
var OrgPolicyAPI = base.APIConfig{
	BaseURL:     "https://orgpolicy.googleapis.com/v2",
	APIVersion:  "v2",
	PathBuilder: orgPolicyPathBuilder,
	// Org Policy uses pageSize/pageToken (rejects the Compute "maxResults" style).
	Pagination: &base.PaginationConfig{PageSizeParam: "pageSize", PageTokenParam: "pageToken"},
}

// OrgPolicyOperations - Org Policy admin operations are synchronous.
// projects.policies.create returns the Policy directly; delete returns Empty.
var OrgPolicyOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractOrgPolicyNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// OrgPolicyNativeID - full resource path "projects/{project}/policies/{constraint}".
var OrgPolicyNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseOrgPolicyNativeID,
}

// orgPolicyPathBuilder builds /projects/{project}/policies[/{constraint}].
// The policy id is the constraint name (e.g. "iam.disableServiceAccountKeyCreation").
func orgPolicyPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/%s", ctx.Project, ctx.ResourceType)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractOrgPolicyNativeID returns the full resource path. Create/Read responses
// carry it in "name" as "projects/{projectNumber}/policies/{constraint}". When
// absent, it is rebuilt from context (project + "policies" + constraint).
func extractOrgPolicyNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name, ok := response["name"].(string); ok && strings.HasPrefix(name, "projects/") {
		return name
	}
	if ctx.ResourceName != "" {
		return fmt.Sprintf("projects/%s/%s/%s", ctx.Project, ctx.ResourceType, ctx.ResourceName)
	}
	return ""
}

// parseOrgPolicyNativeID parses "projects/{project}/policies/{constraint}".
// The constraint may contain dots (e.g. "iam.disableServiceAccountKeyCreation")
// but never a slash, so a 4-segment split is exact.
func parseOrgPolicyNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 4 || parts[0] != "projects" || parts[2] != "policies" {
		return base.PathContext{}, fmt.Errorf("invalid org policy native ID: %s", nativeID)
	}
	return base.PathContext{
		Project:      parts[1],
		ResourceType: parts[2],
		ResourceName: parts[3],
	}, nil
}

// expandNameRequestTransformer rewrites the user-declared short constraint id in
// "name" to the full path the create body requires:
//
//	"iam.disableServiceAccountKeyCreation"
//	  -> "projects/{project}/policies/iam.disableServiceAccountKeyCreation"
//
// Org Policy's projects.policies.create takes no policyId query parameter; the
// constraint is embedded in Policy.name in the request body (the API's own
// naming rule). This mirrors the FullResourceName expander pattern. Already-full
// paths (containing a "/") are left untouched, so the transform is idempotent.
func expandNameRequestTransformer(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		body[k] = v
	}
	if name, ok := body["name"].(string); ok && name != "" && !strings.Contains(name, "/") {
		body["name"] = fmt.Sprintf("projects/%s/policies/%s", ctx.Project, name)
	}
	return body, nil
}
