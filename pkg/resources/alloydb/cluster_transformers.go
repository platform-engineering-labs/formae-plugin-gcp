// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package alloydb

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// networkCanonicalizeTransformer rewrites the project segment of
// networkConfig.network back to the declared project ID.
//
// Users declare the network as "projects/<projectID>/global/networks/<net>",
// but on read-back GCP canonicalizes it to "projects/<projectNumber>/...".
// That mismatch fails Verify. Rewrite the segment after "projects/" to
// ctx.Project so read matches the declaration. No-op when absent or already
// short.
var networkCanonicalizeTransformer = base.ResponseTransformerFunc(
	func(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
		nc, ok := apiResponse["networkConfig"].(map[string]interface{})
		if !ok {
			return apiResponse
		}
		network, ok := nc["network"].(string)
		if !ok || network == "" {
			return apiResponse
		}
		const prefix = "projects/"
		if !strings.HasPrefix(network, prefix) {
			return apiResponse
		}
		// network == "projects/<num>/global/networks/<net>"
		rest := network[len(prefix):]
		if i := strings.Index(rest, "/"); i >= 0 {
			nc["network"] = prefix + ctx.Project + rest[i:]
		}
		return apiResponse
	})

// clusterResponseTransformer canonicalizes the network project segment, then
// shortens the full-path name to its last segment.
var clusterResponseTransformer = &base.CompositeResponseTransformer{
	Transformers: []base.ResponseTransformer{
		networkCanonicalizeTransformer,
		base.ShortNameResponseTransformer,
	},
}

// instanceClusterFromName lifts the owning cluster id out of the instance's
// full resource name. The API response carries no "cluster" field — it is a
// path component — so without this the stored state would not match the
// declared forma.
var instanceClusterFromName = base.ResponseTransformerFunc(
	func(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
		name, ok := apiResponse["name"].(string)
		if !ok {
			return apiResponse
		}
		parts := strings.Split(name, "/")
		for i := 0; i+1 < len(parts); i++ {
			if parts[i] == "clusters" {
				apiResponse["cluster"] = parts[i+1]
				break
			}
		}
		return apiResponse
	})

// instanceResponseTransformer lifts the cluster id out of the name, then
// shortens the name to the instance id.
var instanceResponseTransformer = &base.CompositeResponseTransformer{
	Transformers: []base.ResponseTransformer{
		instanceClusterFromName,
		base.ShortNameResponseTransformer,
	},
}

// userResponseTransformer lifts the owning cluster out of the user's full
// resource name, shortens the name to the user id, and drops the input-only
// secret fields. `password` and `keepExtraRoles` are documented input-only and
// are not returned today; dropping them defensively keeps a password out of
// stored state if that ever changes.
var userResponseTransformer = &base.CompositeResponseTransformer{
	Transformers: []base.ResponseTransformer{
		instanceClusterFromName,
		base.ResponseTransformerFunc(
			func(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
				delete(apiResponse, "password")
				delete(apiResponse, "keepExtraRoles")
				return apiResponse
			}),
		base.ShortNameResponseTransformer,
	},
}

// backupRequestTransformer expands the cluster id into the full resource path
// the API wants. The schema exposes the short id so a forma can reference it
// through a resolvable (which is also how a backup orders itself after the
// instance); "clusterName" is what the wire expects.
var backupRequestTransformer = base.RequestTransformerFunc(
	func(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
		out := make(map[string]interface{}, len(props))
		for k, v := range props {
			if k == "cluster" {
				continue
			}
			out[k] = v
		}
		cluster, ok := props["cluster"].(string)
		if !ok || cluster == "" {
			return out, nil
		}
		if strings.HasPrefix(cluster, "projects/") {
			out["clusterName"] = cluster
			return out, nil
		}
		out["clusterName"] = fmt.Sprintf("projects/%s/locations/%s/clusters/%s",
			ctx.Project, ctx.Location, cluster)
		return out, nil
	})

// backupResponseTransformer folds the full clusterName back to the short id the
// schema declares, then shortens the backup's own name.
var backupResponseTransformer = &base.CompositeResponseTransformer{
	Transformers: []base.ResponseTransformer{
		base.ResponseTransformerFunc(
			func(apiResponse map[string]interface{}, _ base.TransformContext) map[string]interface{} {
				if cn, ok := apiResponse["clusterName"].(string); ok && cn != "" {
					if i := strings.LastIndex(cn, "/"); i >= 0 {
						apiResponse["cluster"] = cn[i+1:]
					}
					delete(apiResponse, "clusterName")
				}
				return apiResponse
			}),
		base.ShortNameResponseTransformer,
	},
}
