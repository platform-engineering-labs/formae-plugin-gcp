// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigtable

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
)

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
