// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package servicedirectory

import (
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// requestTransformer drops the fields that address a resource rather than
// describe it. The id travels as a create-time query parameter and the rest of
// the address is in the URL, so a body carrying them is rejected.
func requestTransformer(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		switch k {
		case "name", "project", "namespace", "service":
			continue
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
