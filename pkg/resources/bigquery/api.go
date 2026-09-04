// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigquery

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// BigQueryAPI - BigQuery API v2, for the collections in this package driven by
// base rather than by the cloud.google.com/go/bigquery client library.
//
// Dataset, Table and Routine predate base and each carries a hand-written
// provisioner over the client library. Nothing about them has to change for a
// config-driven type to sit beside them: both paths end up in the same
// pkg/resources/registry, keyed by resource type and operation.
//
// The paths this builder produces carry no location segment of any kind - a
// dataset's location is one of its properties, not a URL component - so the
// resources below set no ScopeConfig and the target's region never reaches a
// URL. Note the base URL keeps the "/bigquery/v2" prefix; BigQuery is one of the
// older APIs and does not serve at the bare host.
var BigQueryAPI = base.APIConfig{
	BaseURL:     "https://bigquery.googleapis.com/bigquery/v2",
	APIVersion:  "v2",
	PathBuilder: bigQueryPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// BigQueryOperations - synchronous. BigQuery v2 has no Operation resource at
// all: an insert answers with the created resource, an update with the updated
// one, and a delete with an empty body.
//
// The synchronous config is load-bearing rather than tidy. With the async path
// the extractor below finds no operation name, Create reports InProgress with an
// empty RequestID, and formae then polls the bare base URL - which is not an
// operation - forever, on a resource that was in fact created.
var BigQueryOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractBigQueryNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// BigQueryNativeID - the full resource path. For the one type registered here
// that is three collections deep:
//
//	projects/{project}/datasets/{dataset}/tables/{table}/rowAccessPolicies/{policy}
var BigQueryNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseBigQueryNativeID,
}

// bigQueryPathBuilder builds
// /projects/{project}[/datasets/{dataset}][/{parentType}/{parent}]/{resourceType}[/{name}].
//
// The immediate parent comes from ParentResource and the dataset above it from
// CustomSegments[0], which is where base.buildPathContext copies the property
// named by ParentResourceConfig.GrandParentPropertyName.
func bigQueryPathBuilder(ctx base.PathContext) string {
	path := "/projects/" + ctx.Project
	if len(ctx.CustomSegments) > 0 && ctx.CustomSegments[0] != "" {
		path += "/datasets/" + ctx.CustomSegments[0]
	}
	if ctx.ParentType != "" && ctx.ParentResource != "" {
		path += "/" + ctx.ParentType + "/" + ctx.ParentResource
	}
	path += "/" + ctx.ResourceType
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// parseBigQueryNativeID restores the context a read, update or delete needs,
// including both collections above a row access policy.
//
// The default path parser cannot do this: it walks key/value pairs and
// overwrites ResourceType as it goes, so a policy's id would arrive with its
// dataset and table dropped and the request would address
// "/projects/{p}/rowAccessPolicies/{pol}", which 404s.
func parseBigQueryNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 8 || parts[0] != "projects" || parts[2] != "datasets" ||
		parts[4] != "tables" || parts[6] != rowAccessPolicyCollection {
		return base.PathContext{}, fmt.Errorf("invalid bigquery native ID: %s", nativeID)
	}
	return base.PathContext{
		Project:        parts[1],
		CustomSegments: []string{parts[3]},
		ParentType:     parts[4],
		ParentResource: parts[5],
		ResourceType:   parts[6],
		ResourceName:   parts[7],
	}, nil
}

// extractBigQueryNativeID builds the resource path from whatever the caller has.
//
// A row access policy is not identified by a "name" field: its identity is the
// composite rowAccessPolicyReference{projectId,datasetId,tableId,policyId}, and
// that object is the only identity a list item carries. Reading it first is
// therefore what makes the type listable at all - base.extractNativeIDFromItem
// falls back to a "name" that never exists here, and every listed item would
// yield nothing, which is indistinguishable from an empty collection.
func extractBigQueryNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if ref, ok := response[rowAccessPolicyRefField].(map[string]interface{}); ok {
		project, _ := ref["projectId"].(string)
		dataset, _ := ref["datasetId"].(string)
		table, _ := ref["tableId"].(string)
		policy, _ := ref["policyId"].(string)
		if project != "" && dataset != "" && table != "" && policy != "" {
			return fmt.Sprintf("projects/%s/datasets/%s/tables/%s/%s/%s",
				project, dataset, table, rowAccessPolicyCollection, policy)
		}
	}
	// Create has no reference to read until the response arrives, and a failed
	// or partial response still needs a native ID to address the resource by.
	if ctx.ResourceName == "" || ctx.ParentResource == "" ||
		len(ctx.CustomSegments) == 0 || ctx.CustomSegments[0] == "" {
		return ""
	}
	return fmt.Sprintf("projects/%s/datasets/%s/%s/%s/%s/%s",
		ctx.Project, ctx.CustomSegments[0], ctx.ParentType, ctx.ParentResource,
		ctx.ResourceType, ctx.ResourceName)
}
