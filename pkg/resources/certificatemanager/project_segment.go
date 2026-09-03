// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package certificatemanager

import (
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// A forma names another Certificate Manager resource through its resolvable,
// and a resolvable can only yield a property the target declares - for a
// DnsAuthorization or a Certificate that is its short name. The API wants a full
// path on the way in and reports one on the way out, with the project *number*
// where the forma used the project id:
//
//	declared  a
//	sent      projects/development-477117/locations/global/dnsAuthorizations/a
//	reported  projects/989754770009/locations/global/dnsAuthorizations/a
//
// Expanding the request alone is worse than doing nothing: the declared side
// stays short while state holds a path, and because both fields are immutable
// every re-apply plans a *replacement* - which then fails, because the
// certificate is still referenced by the map entry that needs it.
//
// So the request expands and the response shortens, and the two meet at the form
// the forma actually used. Shortening also disposes of the project-number
// problem, since the segment is discarded either way.

// shortenRef reduces a full resource path to its last segment, which is the
// form a resolvable yields. A value that is already short is left alone.
func shortenRef(v string) string {
	if i := strings.LastIndex(v, "/"); i >= 0 {
		return v[i+1:]
	}
	return v
}

// shortenRefList applies it across a list, and reports whether the value was
// one.
func shortenRefList(raw interface{}) (interface{}, bool) {
	list, ok := raw.([]interface{})
	if !ok {
		return raw, false
	}
	out := make([]interface{}, len(list))
	for i, item := range list {
		if s, ok := item.(string); ok {
			out[i] = shortenRef(s)
			continue
		}
		out[i] = item
	}
	return out, true
}

// expandShortRef turns a bare id into the full path the API expects. A forma
// names another resource through its resolvable, and a resolvable can only
// yield a property the target declares - for a DnsAuthorization that is its
// short name. The API wants
// "projects/{p}/locations/global/{collection}/{name}", so the short form is
// expanded on the way out. A value that already carries a path is left alone.
func expandShortRef(v, project, collection string) string {
	if v == "" || strings.Contains(v, "/") || project == "" {
		return v
	}
	return "projects/" + project + "/locations/" + certificateManagerLocation + "/" + collection + "/" + v
}

// expandShortRefList applies it across a list.
func expandShortRefList(raw interface{}, project, collection string) (interface{}, bool) {
	list, ok := raw.([]interface{})
	if !ok {
		return raw, false
	}
	out := make([]interface{}, len(list))
	for i, item := range list {
		if s, ok := item.(string); ok {
			out[i] = expandShortRef(s, project, collection)
			continue
		}
		out[i] = item
	}
	return out, true
}

// The API field is "managed"; the schema calls it "managedCertificate" because
// "managed" is a fixed property of formae.Resource and a resource redeclaring it
// does not evaluate. The two transformers below are the only places that know.
const (
	schemaManagedField = "managedCertificate"
	apiManagedField    = "managed"
)

// certificateRequestTransformer renames managedCertificate to what the API
// calls it, expands the authorizations a forma named by short id, then drops
// what may not be patched.
func certificateRequestTransformer(body map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	out, err := base.DropFieldsOnUpdate("name", schemaManagedField, "selfManaged", "scope").Transform(body, ctx)
	if err != nil {
		return nil, err
	}
	if v, ok := out[schemaManagedField]; ok {
		delete(out, schemaManagedField)
		out[apiManagedField] = v
	}
	if managed, ok := out[apiManagedField].(map[string]interface{}); ok {
		if v, ok := expandShortRefList(managed["dnsAuthorizations"], ctx.Project, "dnsAuthorizations"); ok {
			managed["dnsAuthorizations"] = v
		}
		if s, ok := managed["issuanceConfig"].(string); ok {
			managed["issuanceConfig"] = expandShortRef(s, ctx.Project, "certificateIssuanceConfigs")
		}
	}
	return out, nil
}

// certificateMapEntryRequestTransformer does the same for the certificates an
// entry points at.
func certificateMapEntryRequestTransformer(body map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	// "certificateMap" addresses the entry rather than describing it - it is a
	// path component, and the API rejects it as an unknown body field on create
	// as well as update, so it goes unconditionally. "name" is dropped on update
	// only: create reads the id (?certificateMapEntryId=) from it.
	out, err := (&base.CompositeRequestTransformer{Transformers: []base.RequestTransformer{
		base.DropFields("certificateMap"),
		base.DropFieldsOnUpdate("name", "hostname", "matcher"),
	}}).Transform(body, ctx)
	if err != nil {
		return nil, err
	}
	if v, ok := expandShortRefList(out["certificates"], ctx.Project, "certificates"); ok {
		out["certificates"] = v
	}
	return out, nil
}

// certificateResponseTransformer shortens managed.dnsAuthorizations and
// managed.issuanceConfig, then shortens the full-path name.
var certificateResponseTransformer = &base.CompositeResponseTransformer{
	Transformers: []base.ResponseTransformer{
		base.ResponseTransformerFunc(func(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
			managed, ok := apiResponse[apiManagedField].(map[string]interface{})
			if !ok {
				return apiResponse
			}
			delete(apiResponse, apiManagedField)
			apiResponse[schemaManagedField] = managed

			// Output-only fields the API reports inside "managed". A schema
			// hint is only emitted for a top-level field, so a nested one can
			// never be marked hasProviderDefault - Verify sees it as a property
			// that is neither declared nor defaulted and fails the case. They
			// describe issuance progress, which no forma declares, so drop them.
			for _, k := range []string{"state", "authorizationAttemptInfo", "provisioningIssue"} {
				delete(managed, k)
			}
			if v, ok := shortenRefList(managed["dnsAuthorizations"]); ok {
				managed["dnsAuthorizations"] = v
			}
			if s, ok := managed["issuanceConfig"].(string); ok {
				managed["issuanceConfig"] = shortenRef(s)
			}
			return apiResponse
		}),
		base.ShortNameResponseTransformer,
	},
}

// certificateMapEntryResponseTransformer shortens the certificates it
// points at, then shortens the name and lifts the owning map out of the path.
var certificateMapEntryResponseTransformer = &base.CompositeResponseTransformer{
	Transformers: []base.ResponseTransformer{
		base.ResponseTransformerFunc(func(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
			if v, ok := shortenRefList(apiResponse["certificates"]); ok {
				apiResponse["certificates"] = v
			}
			// The map is a path component, not a body field, so recover it from
			// the name before ShortNameResponseTransformer discards the path:
			// projects/{p}/locations/global/certificateMaps/{map}/certificateMapEntries/{entry}
			if name, ok := apiResponse["name"].(string); ok {
				parts := strings.Split(name, "/")
				if len(parts) == 8 && parts[4] == "certificateMaps" && parts[6] == "certificateMapEntries" {
					apiResponse["certificateMap"] = parts[5]
				}
			}
			return apiResponse
		}),
		base.ShortNameResponseTransformer,
	},
}
