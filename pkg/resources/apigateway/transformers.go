// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package apigateway

import (
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// serverSetFields are reported by the API and never sent back. Putting one in an
// update body would put it in the update mask, which the API refuses.
var serverSetFields = map[string]bool{
	"path":            true,
	"managedService":  true,
	"serviceConfigId": true,
	"defaultHostname": true,
	"state":           true,
	"createTime":      true,
	"updateTime":      true,
}

// requestTransformer drops the fields that address a resource rather than
// describe it, and the ones only the server sets.
//
// "name" stays for a create: base reads body["name"] after this runs to build
// the create-time id parameter and deletes it itself. On an update it has to go,
// because the mask is built from the body and the id is immutable.
func requestTransformer(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		if serverSetFields[k] {
			continue
		}
		switch k {
		case "project", "api":
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
// while a forma declares the short id plus the api it hangs off.
func responseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	out := make(map[string]interface{}, len(apiResponse)+2)
	for k, v := range apiResponse {
		out[k] = v
	}

	name, _ := out["name"].(string)
	parts := strings.Split(name, "/")
	if len(parts) < 6 || parts[0] != "projects" {
		if ctx.Project != "" {
			out["project"] = ctx.Project
		}
		return out
	}

	// The API answers with the project *number* in the path it reports, while a
	// forma names the project by id. Prefer the configured id so the two agree.
	out["project"] = parts[1]
	if ctx.Project != "" {
		out["project"] = ctx.Project
	}
	switch {
	case len(parts) == 6:
		out["name"] = parts[5]
	case len(parts) == 8 && parts[4] == "apis" && parts[6] == "configs":
		out["api"] = parts[5]
		out["name"] = parts[7]
		// A gateway names the config it serves as a full path, and name now
		// holds only the short id, so keep the path a resource can refer to.
		out["path"] = name
	}
	return out
}
