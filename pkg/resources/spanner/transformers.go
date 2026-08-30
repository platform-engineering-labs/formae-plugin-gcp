// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package spanner

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// instanceRequestTransformer builds a CreateInstance body.
//
// Spanner takes the id as instanceId alongside an instance object, rather than
// a name in the body or a query parameter, so the request is assembled here
// instead of using CreateIDParam. The config may be given as a short id, which
// is expanded to the full path the API wants.
func instanceRequestTransformer(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	name, _ := props["name"].(string)
	project, _ := props["project"].(string)
	if project == "" {
		project = ctx.Project
	}

	instance := make(map[string]interface{}, len(props))
	for k, v := range props {
		switch k {
		case "name", "project", "state":
			continue
		}
		instance[k] = v
	}
	if cfg, ok := instance["config"].(string); ok && cfg != "" && !strings.Contains(cfg, "/") {
		instance["config"] = fmt.Sprintf("projects/%s/instanceConfigs/%s", project, cfg)
	}

	return map[string]interface{}{
		"instanceId": name,
		"instance":   instance,
	}, nil
}

// instanceResponseTransformer shortens the full path back to the short id a
// forma declares, and puts the project back.
func instanceResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	out := make(map[string]interface{}, len(apiResponse)+1)
	for k, v := range apiResponse {
		out[k] = v
	}
	if name, ok := out["name"].(string); ok {
		parts := strings.Split(name, "/")
		if len(parts) == 4 && parts[2] == "instances" {
			out["project"] = parts[1]
			out["name"] = parts[3]
		}
	}
	if ctx.Project != "" {
		out["project"] = ctx.Project
	}
	// The config comes back as a full path; a forma names the short id.
	if cfg, ok := out["config"].(string); ok {
		if i := strings.LastIndex(cfg, "/"); i >= 0 {
			out["config"] = cfg[i+1:]
		}
	}
	return out
}

// databaseRequestTransformer builds a CreateDatabase body.
//
// Spanner creates a database by executing a statement rather than by taking a
// name: the id lives inside `CREATE DATABASE`, quoted, and the API rejects a
// body carrying a name field of its own.
func databaseRequestTransformer(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
	name, _ := props["name"].(string)
	body := map[string]interface{}{
		"createStatement": fmt.Sprintf("CREATE DATABASE `%s`", name),
	}
	if dialect, ok := props["databaseDialect"].(string); ok && dialect != "" {
		body["databaseDialect"] = dialect
	}
	return body, nil
}

// databaseResponseTransformer recovers the instance and the short id, both of
// which live only in the path.
func databaseResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	out := make(map[string]interface{}, len(apiResponse)+2)
	for k, v := range apiResponse {
		out[k] = v
	}
	if name, ok := out["name"].(string); ok {
		parts := strings.Split(name, "/")
		if len(parts) == 6 && parts[2] == "instances" && parts[4] == "databases" {
			out["project"] = parts[1]
			out["instance"] = parts[3]
			out["name"] = parts[5]
		}
	}
	if ctx.Project != "" {
		out["project"] = ctx.Project
	}
	return out
}
