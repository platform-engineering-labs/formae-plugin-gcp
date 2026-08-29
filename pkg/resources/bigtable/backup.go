// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigtable

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// backupRequestTransformer prepares a CreateBackup body.
//
// The API wants sourceTable as the full table path, while a forma passes a
// resolvable that resolves to the bare table id - which is what gives formae
// the ordering edge from the backup to the table. project, instance and cluster
// address the backup in the URL and are not body fields.
func backupRequestTransformer(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		switch k {
		case "project", "instance", "cluster":
			continue
		}
		body[k] = v
	}

	instance, _ := props["instance"].(string)
	project, _ := props["project"].(string)
	if project == "" {
		project = ctx.Project
	}
	if table, ok := body["sourceTable"].(string); ok && table != "" && !strings.Contains(table, "/") {
		body["sourceTable"] = fmt.Sprintf("projects/%s/instances/%s/tables/%s", project, instance, table)
	}
	return body, nil
}

// backupResponseTransformer is the mirror: it shortens the backup's own name and
// its sourceTable, and recovers the instance and cluster, which live only in the
// path.
func backupResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	out := base.AddProjectResponseTransformer.Transform(apiResponse, ctx)

	if table, ok := out["sourceTable"].(string); ok {
		if i := strings.LastIndex(table, "/tables/"); i >= 0 {
			out["sourceTable"] = table[i+len("/tables/"):]
		}
	}

	name, ok := out["name"].(string)
	if !ok {
		return out
	}
	parts := strings.Split(name, "/")
	// projects/{p}/instances/{i}/clusters/{c}/backups/{b}
	if len(parts) == 8 && parts[2] == "instances" && parts[4] == "clusters" && parts[6] == "backups" {
		out["instance"] = parts[3]
		out["cluster"] = parts[5]
		out["name"] = parts[7]
	}
	return out
}

// materializedViewRequestTransformer drops the fields that address the view in
// the URL. The rest of the body is what the API takes: a name, a query and
// deletionProtection.
func materializedViewRequestTransformer(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		switch k {
		case "project", "instance":
			continue
		}
		body[k] = v
	}
	return body, nil
}

// materializedViewResponseTransformer shortens the name and recovers the
// instance, which lives only in the path.
func materializedViewResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	out := base.AddProjectResponseTransformer.Transform(apiResponse, ctx)

	name, ok := out["name"].(string)
	if !ok {
		return out
	}
	parts := strings.Split(name, "/")
	// projects/{p}/instances/{i}/materializedViews/{v}
	if len(parts) == 6 && parts[2] == "instances" && parts[4] == "materializedViews" {
		out["instance"] = parts[3]
		out["name"] = parts[5]
	}
	return out
}
