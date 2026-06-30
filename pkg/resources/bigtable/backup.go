// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigtable

import (
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
)

// backupResponseTransformer reconstructs the declared input fields from the
// backup's full resource name. The API returns
// name="projects/{p}/instances/{i}/clusters/{c}/backups/{b}" and does NOT echo
// instance/cluster, so derive them (and the short name) for state matching.
// sourceTable and expireTime are left as the API returns them.
func backupResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	apiResponse["project"] = ctx.Project
	if name := utils.GetString(apiResponse, "name"); name != "" {
		parts := strings.Split(name, "/")
		for i := 0; i+1 < len(parts); i++ {
			switch parts[i] {
			case "instances":
				apiResponse["instance"] = parts[i+1]
			case "clusters":
				apiResponse["cluster"] = parts[i+1]
			case "backups":
				apiResponse["name"] = parts[i+1]
			}
		}
	}
	return apiResponse
}

// backupBodyBuilder builds the request body for creating a Bigtable backup.
// project/instance/cluster/name are carried via the path and the backup_id
// query parameter; only sourceTable and expireTime belong in the body.
func backupBodyBuilder(props map[string]interface{}) (map[string]interface{}, error) {
	body := make(map[string]interface{})

	if sourceTable := utils.GetString(props, "sourceTable"); sourceTable != "" {
		body["sourceTable"] = sourceTable
	}
	if expireTime := utils.GetString(props, "expireTime"); expireTime != "" {
		body["expireTime"] = expireTime
	}

	return body, nil
}
