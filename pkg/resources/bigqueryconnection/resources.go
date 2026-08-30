// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package bigqueryconnection implements the BigQuery Connection API, which is a
// separate service from BigQuery itself: a different host, and a resource path
// that is location-based rather than dataset-based.
package bigqueryconnection

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const ConnectionResourceType = "GCP::BigQuery::Connection"

// ConnectionAPI - BigQuery Connection API v1. Every operation is synchronous.
var ConnectionAPI = base.APIConfig{
	BaseURL:     "https://bigqueryconnection.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: connectionPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// ConnectionOperations - synchronous, so Status has nothing to poll.
var ConnectionOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      extractConnectionNativeID,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// ConnectionNativeID - the full path the API reports as "name":
// projects/{project}/locations/{location}/connections/{connection}.
var ConnectionNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseConnectionNativeID,
}

func connectionPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s/connections", ctx.Project, ctx.Location)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractConnectionNativeID takes the full path the API reports.
func extractConnectionNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name := utils.GetString(response, "name"); name != "" {
		return name
	}
	if ctx.ResourceName == "" {
		return ""
	}
	return strings.TrimPrefix(connectionPathBuilder(ctx), "/")
}

func parseConnectionNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "connections" {
		return base.PathContext{}, fmt.Errorf("invalid bigquery connection native ID: %s", nativeID)
	}
	return base.PathContext{
		Project:      parts[1],
		Location:     parts[3],
		ResourceType: "connections",
		ResourceName: parts[5],
	}, nil
}

// requestTransformer drops what addresses the connection rather than describing
// it. The id travels as a create-time query parameter, which base moves out of
// the body itself, so "name" stays for a create and goes on an update.
func requestTransformer(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		switch k {
		case "project", "location", "hasCredential", "cloudResourceServiceAccountId":
			continue
		case "name":
			if ctx.Operation == resource.OperationUpdate {
				continue
			}
		}
		body[k] = v
	}
	return body, nil
}

// responseTransformer is the mirror: the API reports the full path as "name",
// while a forma declares the short id plus the location it lives in.
func responseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	out := make(map[string]interface{}, len(apiResponse)+2)
	for k, v := range apiResponse {
		out[k] = v
	}
	// The service account BigQuery mints is reported inside cloudResource, but a
	// hint is only emitted for a top-level field, so a nested one cannot be
	// marked as server-filled and reads as a property the forma never declared.
	// Lift it out and leave the selector empty, which is all a create sends.
	if cr, ok := out["cloudResource"].(map[string]interface{}); ok {
		if sa, ok := cr["serviceAccountId"].(string); ok && sa != "" {
			out["cloudResourceServiceAccountId"] = sa
		}
		out["cloudResource"] = map[string]interface{}{}
	}

	name, _ := out["name"].(string)
	parts := strings.Split(name, "/")
	if len(parts) == 6 && parts[0] == "projects" && parts[2] == "locations" {
		// The path reports the project *number*, while a forma names the project
		// by id, so the configured id wins where there is one.
		out["project"] = parts[1]
		out["location"] = parts[3]
		out["name"] = parts[5]
	}
	if ctx.Project != "" {
		out["project"] = ctx.Project
	}
	return out
}

var connectionRegistry *base.ResourceRegistry

func init() {
	connectionRegistry = base.NewResourceRegistry(ConnectionAPI, ConnectionOperations, ConnectionNativeID)

	err := connectionRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: ConnectionResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "connections",
				Scope:              &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam:      "connectionId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
				ListItemsKey:       "connections",
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationUpdate,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			RequestTransformer:  base.RequestTransformerFunc(requestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(responseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}
}
