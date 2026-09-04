// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package networkservices

import (
	"fmt"
	"strings"
)

// expandGlobalRef turns the short name a resolvable yields ("my-mesh") into the
// full path this API requires
// ("projects/{project}/locations/global/{collection}/my-mesh").
//
// The short form is not merely unconventional here, it is refused. Verified
// live against an httpRoute: PATCH with meshes ["formae-probe-mesh"] answers
// 400 INVALID_ARGUMENT, "mesh formae-probe-mesh referenced by HttpRoute ...".
// Only the full path is accepted.
//
// The location is fixed to global rather than read from the context because
// every collection referenced this way - meshes, and the three Network
// Security policy kinds an endpoint policy names - is itself global. A regional
// segment here would address nothing.
//
// A value that already contains a "/" is passed through untouched, so a forma
// may name a resource in another project, or give a full compute URL, and have
// it survive.
func expandGlobalRef(value, project, collection string) string {
	if value == "" || strings.Contains(value, "/") {
		return value
	}
	return fmt.Sprintf("projects/%s/locations/%s/%s/%s", project, defaultLocation, collection, value)
}

// shortenRef is the exact inverse: it reduces the full path the API reports
// back to the short name the forma declared.
//
// Both halves are required. Expanding on the request without shortening on the
// response leaves the declared value and the stored state permanently
// disagreeing, and every re-apply then plans a replacement of a resource that
// has not changed. expand(shorten(x)) == x is pinned as an identity in
// refs_test.go.
func shortenRef(value string) string {
	if i := strings.LastIndex(value, "/"); i >= 0 {
		return value[i+1:]
	}
	return value
}

// expandRefList expands every entry of a reference listing in place, returning
// a new slice. The value arrives as []interface{} because it has been through
// JSON, so each entry is type-asserted rather than assumed.
func expandRefList(value interface{}, project, collection string) interface{} {
	list, ok := value.([]interface{})
	if !ok {
		return value
	}
	out := make([]interface{}, len(list))
	for i, item := range list {
		if s, ok := item.(string); ok {
			out[i] = expandGlobalRef(s, project, collection)
			continue
		}
		out[i] = item
	}
	return out
}

// shortenRefList is the inverse of expandRefList.
func shortenRefList(value interface{}) interface{} {
	list, ok := value.([]interface{})
	if !ok {
		return value
	}
	out := make([]interface{}, len(list))
	for i, item := range list {
		if s, ok := item.(string); ok {
			out[i] = shortenRef(s)
			continue
		}
		out[i] = item
	}
	return out
}
