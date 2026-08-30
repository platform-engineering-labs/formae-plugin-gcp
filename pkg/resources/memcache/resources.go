// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package memcache implements Memorystore for Memcached.
//
// A memcached instance is billed by node-hour for as long as it exists and
// takes twenty minutes or more to create, which is why its conformance case is
// among the slowest here.
package memcache

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const InstanceResourceType = "GCP::Memcache::Instance"

// MemcacheAPI - Memorystore for Memcached v1. Writes are long-running.
var MemcacheAPI = base.APIConfig{
	BaseURL:     "https://memcache.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: memcachePathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// MemcacheOperations - create, patch and delete answer with an Operation.
var MemcacheOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   func(r map[string]interface{}) string { return utils.GetString(r, "name") },
	OperationURLBuilder:    func(_ base.PathContext, operationID string) string { return operationID },
	NativeIDExtractor:      extractMemcacheNativeID,
	OperationStatusChecker: base.CheckLROStatus,
}

// MemcacheNativeID - projects/{project}/locations/{location}/instances/{name}.
var MemcacheNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseMemcacheNativeID,
}

func memcachePathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s/instances", ctx.Project, ctx.Location)
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractMemcacheNativeID reads the resource path. A fresh operation does not
// carry the resource, but its metadata names the target it is building.
func extractMemcacheNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name := utils.GetString(response, "name"); name != "" && !strings.Contains(name, "/operations/") {
		return name
	}
	if metadata, ok := response["metadata"].(map[string]interface{}); ok {
		if target := utils.GetString(metadata, "target"); target != "" {
			return target
		}
	}
	if ctx.ResourceName == "" {
		return ""
	}
	return strings.TrimPrefix(memcachePathBuilder(ctx), "/")
}

func parseMemcacheNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "instances" {
		return base.PathContext{}, fmt.Errorf("invalid memcache native ID: %s", nativeID)
	}
	return base.PathContext{
		Project: parts[1], Location: parts[3],
		ResourceType: "instances", ResourceName: parts[5],
	}, nil
}

// serverSet are reported by the API and never sent back.
var serverSet = map[string]bool{
	"state":               true,
	"discoveryEndpoint":   true,
	"createTime":          true,
	"updateTime":          true,
	"memcacheNodes":       true,
	"memcacheFullVersion": true,
}

// requestTransformer drops what addresses the instance rather than describing
// it. The id travels as a create-time query parameter, which base moves out of
// the body, so name stays for a create and goes on an update. The authorized
// network may be named short and is expanded to the full path.
func requestTransformer(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	project, _ := props["project"].(string)
	if project == "" {
		project = ctx.Project
	}
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		if serverSet[k] {
			continue
		}
		switch k {
		case "project", "region":
			continue
		case "name":
			if ctx.Operation == resource.OperationUpdate {
				continue
			}
		}
		body[k] = v
	}
	if n, ok := body["authorizedNetwork"].(string); ok && n != "" && !strings.Contains(n, "/") {
		body["authorizedNetwork"] = fmt.Sprintf("projects/%s/global/networks/%s", project, n)
	}
	return body, nil
}

// responseTransformer is the mirror: the API reports the full path as name,
// while a forma declares the short id plus the region it runs in.
func responseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	out := make(map[string]interface{}, len(apiResponse)+2)
	for k, v := range apiResponse {
		out[k] = v
	}
	if name, ok := out["name"].(string); ok {
		parts := strings.Split(name, "/")
		if len(parts) == 6 && parts[2] == "locations" && parts[4] == "instances" {
			out["project"] = parts[1]
			out["region"] = parts[3]
			out["name"] = parts[5]
		}
	}
	if ctx.Project != "" {
		out["project"] = ctx.Project
	}
	// The network comes back as a full path; a forma may name it short.
	if n, ok := out["authorizedNetwork"].(string); ok {
		if i := strings.LastIndex(n, "/"); i >= 0 {
			out["authorizedNetwork"] = n[i+1:]
		}
	}
	return out
}

var memcacheRegistry *base.ResourceRegistry

func init() {
	memcacheRegistry = base.NewResourceRegistry(MemcacheAPI, MemcacheOperations, MemcacheNativeID)

	err := memcacheRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: InstanceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "instances",
				Scope:              &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam:      "instanceId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
				ListItemsKey:       "instances",
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
