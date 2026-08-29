// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package servicedirectory

import (
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// requestTransformer drops the fields that address a resource rather than
// describe it: the parents and the project are all in the URL, so a body
// carrying them is rejected.
//
// "name" is deliberately left in. The id does travel as a create-time query
// parameter rather than in the body, but base is what moves it: it reads
// body["name"] after this transformer runs and deletes it itself. Dropping it
// here left the parameter empty and the API answered "Invalid namespace name:".
func requestTransformer(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		switch k {
		case "project", "namespace", "service":
			continue
		case "uid":
			// Server-set concurrency token; sending it back puts it in the
			// update mask, which the API will not accept.
			continue
		case "name":
			// The update mask is built from the body, and the id is immutable:
			// "Invalid update_mask. Cannot update the name of a namespace."
			if ctx.Operation == resource.OperationUpdate {
				continue
			}
		}
		body[k] = v
	}
	return body, nil
}

// responseTransformer is the mirror. The API reports the full path as "name";
// a forma declares the short id plus the parents it hangs off, so the path is
// taken apart and each piece put back under the name the schema uses.
func responseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	out := make(map[string]interface{}, len(apiResponse)+4)
	for k, v := range apiResponse {
		out[k] = v
	}

	name, _ := out["name"].(string)
	parts := strings.Split(name, "/")
	if len(parts) < 6 || parts[0] != "projects" || parts[4] != "namespaces" {
		if ctx.Project != "" {
			out["project"] = ctx.Project
		}
		return out
	}

	out["project"] = parts[1]
	switch {
	case len(parts) == 6:
		out["name"] = parts[5]
	case len(parts) == 8 && parts[6] == "services":
		out["namespace"] = parts[5]
		out["name"] = parts[7]
	case len(parts) == 10 && parts[6] == "services" && parts[8] == "endpoints":
		out["namespace"] = parts[5]
		out["service"] = parts[7]
		out["name"] = parts[9]
	}
	return out
}
