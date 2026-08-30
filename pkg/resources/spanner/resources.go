// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package spanner implements GCP Spanner resources.
//
// Spanner is the one service in this plugin whose resources are billed for as
// long as they exist: the smallest regional instance is 100 processing units.
// A forma that declares an instance is spending money until it is destroyed.
package spanner

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const (
	InstanceResourceType = "GCP::Spanner::Instance"
	DatabaseResourceType = "GCP::Spanner::Database"
)

// SpannerAPI - Cloud Spanner Admin API v1. Creates are long-running operations.
var SpannerAPI = base.APIConfig{
	BaseURL:     "https://spanner.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: spannerPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// SpannerOperations - create and update answer with an Operation to poll.
var SpannerOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   func(r map[string]interface{}) string { return utils.GetString(r, "name") },
	OperationURLBuilder:    func(_ base.PathContext, operationID string) string { return operationID },
	NativeIDExtractor:      extractSpannerNativeID,
	OperationStatusChecker: base.CheckLROStatus,
}

// SpannerNativeID - the full path the API reports as "name":
//
//	projects/{project}/instances/{instance}[/databases/{database}]
var SpannerNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseSpannerNativeID,
}

func spannerPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s", ctx.Project)
	if ctx.ParentResource != "" {
		path += "/instances/" + ctx.ParentResource
	}
	path += "/" + ctx.ResourceType
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractSpannerNativeID reads the resource path. A fresh operation does not
// carry the resource, but its name embeds the target it is building:
// projects/{p}/instances/{i}/operations/{op}.
func extractSpannerNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name := utils.GetString(response, "name"); name != "" && !strings.Contains(name, "/operations/") {
		return name
	}
	if resp, ok := response["response"].(map[string]interface{}); ok {
		if name := utils.GetString(resp, "name"); name != "" {
			return name
		}
	}
	if name := utils.GetString(response, "name"); strings.Contains(name, "/operations/") {
		if i := strings.Index(name, "/operations/"); i > 0 {
			base := name[:i]
			// An instance operation's prefix is the instance itself; a database
			// operation's prefix is the database.
			if ctx.ResourceType == "databases" && !strings.Contains(base, "/databases/") {
				return ""
			}
			return base
		}
	}
	if ctx.ResourceName == "" {
		return ""
	}
	return strings.TrimPrefix(spannerPathBuilder(ctx), "/")
}

func parseSpannerNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	switch {
	case len(parts) == 4 && parts[0] == "projects" && parts[2] == "instances":
		return base.PathContext{Project: parts[1], ResourceType: "instances", ResourceName: parts[3]}, nil
	case len(parts) == 6 && parts[0] == "projects" && parts[2] == "instances" && parts[4] == "databases":
		return base.PathContext{
			Project: parts[1], ParentResource: parts[3],
			ResourceType: "databases", ResourceName: parts[5],
		}, nil
	}
	return base.PathContext{}, fmt.Errorf("invalid spanner native ID: %s", nativeID)
}

var spannerRegistry *base.ResourceRegistry

func crudOperations() []resource.Operation {
	return []resource.Operation{
		resource.OperationCreate,
		resource.OperationRead,
		resource.OperationUpdate,
		resource.OperationDelete,
		resource.OperationList,
		resource.OperationCheckStatus,
	}
}

func init() {
	spannerRegistry = base.NewResourceRegistry(SpannerAPI, SpannerOperations, SpannerNativeID)

	err := spannerRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: InstanceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "instances",
				SupportsUpdate: false,
				ListItemsKey:   "instances",
			},
			Operations:          crudOperations(),
			RequestTransformer:  base.RequestTransformerFunc(instanceRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(instanceResponseTransformer),
		},
		{
			ResourceType: DatabaseResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "databases",
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "instances",
					PropertyName:   "instance",
					RequiresParent: true,
				},
				SupportsUpdate: false,
				ListItemsKey:   "databases",
			},
			Operations:          crudOperations(),
			RequestTransformer:  base.RequestTransformerFunc(databaseRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(databaseResponseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}
}
