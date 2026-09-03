// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigquery

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

const (
	RowAccessPolicyResourceType = "GCP::BigQuery::RowAccessPolicy"
)

// rowAccessPolicyCollection is the API collection name, and also the key the
// list response holds its array under. base.parseListResponse tries "items"
// first and then the configured ResourceType, so the array is already found by
// the resource-type branch and ResourceConfig.ListItemsKey - checked only after
// it - would never be reached. Naming the constant is the honest way to record
// that the key is not "items".
const rowAccessPolicyCollection = "rowAccessPolicies"

// rowAccessPolicyRefField is the composite identity object. BigQuery puts the
// whole identity here rather than in a "name": every request body must carry it
// and every response echoes it.
const rowAccessPolicyRefField = "rowAccessPolicyReference"

var bigQueryRegistry *base.ResourceRegistry

func init() {
	bigQueryRegistry = base.NewResourceRegistry(BigQueryAPI, BigQueryOperations, BigQueryNativeID)

	err := bigQueryRegistry.RegisterAll([]base.ResourceDefinition{
		{
			// Row-level access control on one table: a SQL boolean expression
			// that decides which rows a principal may see, so a shared table
			// can serve each tenant only its own rows without a view per
			// tenant. Metadata only - a policy stores nothing and BigQuery
			// bills bytes stored and bytes scanned.
			ResourceType: RowAccessPolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: rowAccessPolicyCollection,
				ParentResource: &base.ParentResourceConfig{
					ParentType:              "tables",
					PropertyName:            "tableId",
					RequiresParent:          true,
					GrandParentType:         "datasets",
					GrandParentPropertyName: "datasetId",
				},
				SupportsUpdate: true,
				// PUT, not PATCH: BigQuery v2 offers rowAccessPolicies.update
				// as a full replacement and nothing else. Verified live - a PUT
				// changing only filterPredicate returns the new predicate and
				// the next GET agrees.
				UpdateMethod: base.UpdateMethodPut,
				// Deliberately no UpdateMaskFromBody. The update method takes no
				// updateMask parameter, and sending one is not ignored: the API
				// answers 400 - Unknown name "updateMask": Cannot bind query
				// parameter. Field "updateMask" could not be found in request
				// message - and the update never happens.
				//
				// force on delete is required, not defensive. Dropping the last
				// policy on a table without it answers 400 "Dropping the last
				// row access policy would make the table accessible to all
				// users who have access to the table. If this is intended,
				// please set force to true." - which is precisely what a destroy
				// intends, and without the flag every teardown of a
				// single-policy table fails. It is accepted when other policies
				// remain, and a missing policy still 404s (which base treats as
				// a successful delete) whether it is sent or not.
				DeleteQueryParams: map[string]string{"force": "true"},
			},
			RequestTransformer:  base.RequestTransformerFunc(rowAccessPolicyRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(rowAccessPolicyResponseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}

	// Replaces only the List entry; see row_access_policy_list.go for why the
	// generic one cannot find a policy that no caller names a table for. Called
	// from here rather than from an init of its own because Go runs init
	// functions in filename order: "row_access_policy_list.go" sorts after
	// "resources.go", so an override registered there would win by accident
	// rather than by intent, and reordering the files would silently undo it.
	registerRowAccessPolicyListWalker()
}
