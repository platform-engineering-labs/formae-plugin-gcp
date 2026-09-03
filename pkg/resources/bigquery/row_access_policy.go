// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigquery

import (
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// rowAccessPolicyIdentityFields are the flat properties a forma declares to
// identify a policy. All four are URL components, none is a body field, and
// BigQuery rejects every one of them outright if it is left in the body:
//
//	400 Invalid JSON payload received. Unknown name "name" at
//	    'row_access_policy': Cannot find field.
//
// (and the same for project, datasetId and tableId). They are removed on create
// as well as on update, hence the plain removal rather than
// base.DropFieldsOnUpdate.
var rowAccessPolicyIdentityFields = []string{"project", "datasetId", "tableId", "name"}

// rowAccessPolicyRequestTransformer replaces the four flat identity properties
// with the composite object BigQuery requires.
//
// A row access policy is the one type in this plugin whose identity is not a
// name string. It is
// rowAccessPolicyReference{projectId,datasetId,tableId,policyId}, it is
// Required on both insert and update, and the API checks it against the URL
// segment by segment:
//
//	omitting it entirely  -> 400 "Project ID in URL path and content do not match."
//	a policyId that drifts -> 400 "Policy ID in URL path and content do not match."
//
// So the reference has to be assembled for update as well as for create, from
// the same properties the URL was built from. The project is taken from the
// transform context rather than from the properties for exactly that reason: it
// is the project the URL already names, so the two can never disagree.
//
// This is the request half of the round trip rowAccessPolicyResponseTransformer
// completes. Both halves must exist: the identity fields are immutable, so a
// declared value that state never gets back would plan a replacement on every
// re-apply, and the replacement would then fail.
func rowAccessPolicyRequestTransformer(
	props map[string]interface{}, ctx base.TransformContext,
) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(props)+1)
	drop := make(map[string]bool, len(rowAccessPolicyIdentityFields))
	for _, f := range rowAccessPolicyIdentityFields {
		drop[f] = true
	}
	for k, v := range props {
		if drop[k] {
			continue
		}
		out[k] = v
	}

	dataset, _ := props["datasetId"].(string)
	table, _ := props["tableId"].(string)
	policy, _ := props["name"].(string)
	project := ctx.Project
	if project == "" {
		project, _ = props["project"].(string)
	}

	// Refusing here turns a confusing 400 about mismatched URL and content into
	// a plain statement of what is missing.
	for field, value := range map[string]string{
		"project":   project,
		"datasetId": dataset,
		"tableId":   table,
		"name":      policy,
	} {
		if value == "" {
			return nil, fmt.Errorf(
				"%s is required to address a row access policy and was not supplied", field)
		}
	}

	out[rowAccessPolicyRefField] = map[string]interface{}{
		"projectId": project,
		"datasetId": dataset,
		"tableId":   table,
		"policyId":  policy,
	}
	return out, nil
}

// rowAccessPolicyOutputOnlyFields are the fields BigQuery adds to a response and
// this type does not declare.
//
// They are stripped rather than declared with hasProviderDefault for a reason
// that showed up in the live round trip: an update *resets* creationTime.
// Measured against the API, a policy created at 15:07:31.271482Z and then
// updated reported creationTime 15:07:51.127627Z - equal to its new
// lastModifiedTime. A stored provider default that silently changes on every
// update reads as drift on the next sync, so neither timestamp belongs in
// state. etag is documented as Output only and was never actually returned by
// insert, update, get or list; it is listed here so that if BigQuery starts
// sending it the type does not begin failing verification.
var rowAccessPolicyOutputOnlyFields = []string{
	rowAccessPolicyRefField, "creationTime", "lastModifiedTime", "etag",
}

// rowAccessPolicyResponseTransformer flattens the composite identity back into
// the four properties a forma declared, and drops the output-only fields.
//
// The reference object itself must not survive: it is not a declared property,
// so leaving it in makes verification reject the resource for reporting a field
// that is neither expected nor a provider default.
//
// Note what is deliberately absent: grantees. See rowAccessPolicy.pkl - the API
// accepts it on insert and never returns it from get or list, so it cannot be
// declared, and there is nothing to put back here.
func rowAccessPolicyResponseTransformer(
	apiResponse map[string]interface{}, _ base.TransformContext,
) map[string]interface{} {
	drop := make(map[string]bool, len(rowAccessPolicyOutputOnlyFields))
	for _, f := range rowAccessPolicyOutputOnlyFields {
		drop[f] = true
	}

	out := make(map[string]interface{}, len(apiResponse)+4)
	for k, v := range apiResponse {
		if drop[k] {
			continue
		}
		out[k] = v
	}

	ref, ok := apiResponse[rowAccessPolicyRefField].(map[string]interface{})
	if !ok {
		return out
	}
	for responseKey, propertyKey := range map[string]string{
		"projectId": "project",
		"datasetId": "datasetId",
		"tableId":   "tableId",
		"policyId":  "name",
	} {
		if value, _ := ref[responseKey].(string); value != "" {
			out[propertyKey] = value
		}
	}
	return out
}
