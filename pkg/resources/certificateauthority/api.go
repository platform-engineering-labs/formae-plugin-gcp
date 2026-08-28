// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package certificateauthority

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// CertificateAuthorityAPI - Certificate Authority Service (privateca) API v1.
// Pools and certificate templates are location-scoped; certificate authorities
// are nested under a pool. create/delete are long-running operations (return an
// Operation to poll); get/list return the resource directly.
var CertificateAuthorityAPI = base.APIConfig{
	BaseURL:     "https://privateca.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: certificateAuthorityPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// CertificateAuthorityOperations - asynchronous (LRO). create/delete return an
// Operation; formae polls Status() until the operation reports done.
var CertificateAuthorityOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractCaPoolNativeID,
	OperationStatusChecker: checkOperationStatus,
}

// CertificateAuthorityNativeID - full path, either
// "projects/{p}/locations/{l}/caPools/{name}" or the nested
// "projects/{p}/locations/{l}/caPools/{pool}/certificateAuthorities/{name}".
var CertificateAuthorityNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parsePrivateCANativeID,
}

// parsePrivateCANativeID handles the location-scoped form (6 segments: pools
// and certificate templates) and the nested form (8 segments: a certificate
// authority inside a pool).
func parsePrivateCANativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" {
		return base.PathContext{}, fmt.Errorf("invalid privateca native ID: %s", nativeID)
	}
	switch len(parts) {
	case 6:
		return base.PathContext{
			Project:      parts[1],
			Location:     parts[3],
			ResourceType: parts[4],
			ResourceName: parts[5],
		}, nil
	case 8:
		return base.PathContext{
			Project:        parts[1],
			Location:       parts[3],
			ParentType:     parts[4],
			ParentResource: parts[5],
			ResourceType:   parts[6],
			ResourceName:   parts[7],
		}, nil
	default:
		return base.PathContext{}, fmt.Errorf(
			"invalid privateca native ID: %s (expected 6 or 8 path segments, got %d)", nativeID, len(parts))
	}
}

// nestedInPool are the collections that only exist underneath a caPool.
var nestedInPool = map[string]bool{"certificateAuthorities": true}

// certificateAuthorityPathBuilder builds
//
//	/projects/{p}/locations/{l}/{resourceType}[/{name}]
//	/projects/{p}/locations/{l}/caPools/{pool}/{resourceType}[/{name}]
func certificateAuthorityPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s", ctx.Project, ctx.Location)
	switch {
	case ctx.ParentType != "" && ctx.ParentResource != "":
		path = fmt.Sprintf("%s/%s/%s", path, ctx.ParentType, ctx.ParentResource)
	case ctx.IsList && nestedInPool[ctx.ResourceType]:
		// Discovery lists with no parent to name. privateca accepts "-" in the
		// pool position, so ask across every pool rather than building a URL
		// with an empty segment - which 404s, and made certificate authorities
		// undiscoverable.
		path += "/caPools/-"
	}
	path += "/" + ctx.ResourceType
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractOperationName returns the LRO operation name from a create/delete
// response ("projects/{p}/locations/{l}/operations/{op}"). base.Status GETs
// BaseURL + "/" + this to poll.
func extractOperationName(response map[string]interface{}) string {
	if name, ok := response["name"].(string); ok && strings.Contains(name, "/operations/") {
		return name
	}
	return ""
}

// extractCaPoolNativeID builds the resource path. On async create the response
// is an Operation (not the resource), so build from context — where
// buildPathContext has already set ResourceName from the declared id. Fall back
// to the operation's metadata.target.
func extractCaPoolNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if ctx.ResourceName != "" {
		prefix := fmt.Sprintf("projects/%s/locations/%s", ctx.Project, ctx.Location)
		if ctx.ParentType != "" && ctx.ParentResource != "" {
			prefix = fmt.Sprintf("%s/%s/%s", prefix, ctx.ParentType, ctx.ParentResource)
		}
		return fmt.Sprintf("%s/%s/%s", prefix, ctx.ResourceType, ctx.ResourceName)
	}
	if md, ok := response["metadata"].(map[string]interface{}); ok {
		if target, ok := md["target"].(string); ok {
			if i := strings.Index(target, "projects/"); i >= 0 {
				return target[i:]
			}
		}
	}
	// Direct resource response (get): "name" is the full path.
	if name, ok := response["name"].(string); ok && !strings.Contains(name, "/operations/") {
		if i := strings.Index(name, "projects/"); i >= 0 {
			return name[i:]
		}
	}
	return ""
}

// checkOperationStatus reports whether a polled Operation is done, mapping a
// present "error" to a terminal failure.
func checkOperationStatus(op map[string]interface{}) (bool, error) {
	done, _ := op["done"].(bool)
	if !done {
		return false, nil
	}
	if errObj, ok := op["error"].(map[string]interface{}); ok {
		msg, _ := errObj["message"].(string)
		if msg == "" {
			msg = "operation failed"
		}
		return true, fmt.Errorf("%s", msg)
	}
	return true, nil
}

// dropCAPathFields removes the fields that address the resource in the URL and
// are not body fields. "name" stays: base.Create reads the create id
// (?certificateAuthorityId=, ?certificateTemplateId=) out of it and deletes it
// from the body itself.
func dropCAPathFields(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		switch k {
		case "location", "caPool":
			continue
		}
		body[k] = v
	}
	return body, nil
}

// locationResponseTransformer puts back the short name and the location. The
// API reports "name" as a full path and never reports "location" as a field of
// its own, but a forma declares it - and a declared field the read never
// reports would look like it went missing on every comparison.
func locationResponseTransformer(collection string) base.ResponseTransformerFunc {
	return func(props map[string]interface{}, _ base.TransformContext) map[string]interface{} {
		out := make(map[string]interface{}, len(props)+1)
		for k, v := range props {
			out[k] = v
		}
		name, ok := props["name"].(string)
		if !ok {
			return out
		}
		parts := strings.Split(name, "/")
		if len(parts) == 6 && parts[2] == "locations" && parts[4] == collection {
			out["location"] = parts[3]
			out["name"] = parts[5]
		}
		return out
	}
}

// caResponseTransformer is the nested equivalent: a CA's path also carries the
// pool it lives in.
func caResponseTransformer(props map[string]interface{}, _ base.TransformContext) map[string]interface{} {
	out := make(map[string]interface{}, len(props)+2)
	for k, v := range props {
		out[k] = v
	}
	name, ok := props["name"].(string)
	if !ok {
		return out
	}
	parts := strings.Split(name, "/")
	// projects/{p}/locations/{l}/caPools/{pool}/certificateAuthorities/{name}
	if len(parts) == 8 && parts[2] == "locations" && parts[6] == "certificateAuthorities" {
		out["location"] = parts[3]
		out["caPool"] = parts[5]
		out["name"] = parts[7]
	}
	return out
}
