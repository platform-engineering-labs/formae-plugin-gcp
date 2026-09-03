// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package certificatemanager implements GCP Certificate Manager resources.
package certificatemanager

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// CertificateManagerAPI - Certificate Manager v1. Everything is location-scoped
// and create/delete are long-running operations.
var CertificateManagerAPI = base.APIConfig{
	BaseURL:     "https://certificatemanager.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: certificateManagerPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

// CertificateManagerOperations - asynchronous. create/delete answer with an
// Operation; formae polls Status until it reports done.
var CertificateManagerOperations = base.OperationConfig{
	Synchronous:            false,
	OperationIDExtractor:   extractOperationName,
	OperationURLBuilder:    func(_ base.PathContext, opID string) string { return opID },
	NativeIDExtractor:      extractCertificateManagerNativeID,
	OperationStatusChecker: checkOperationStatus,

	// A certificate's delete is a long-running operation, so the certificate can
	// still be referencing its authorization when the authorization's own delete
	// arrives - Destroy walks the dependency order, it does not wait for the
	// API to finish. The answer is
	//
	//	can't delete dns authorization that is referenced by a certificate
	//
	// which is a race, not a refusal: it stops being true as soon as the
	// certificate is gone. Reporting it as retryable lets formae re-run the
	// delete until it succeeds, the same way Cloud SQL handles a database whose
	// sessions have not yet been reaped.
	RetryableError: func(err error) bool {
		return err != nil && strings.Contains(err.Error(), "referenced by a certificate")
	},
}

// CertificateManagerNativeID - the full resource path, in one of two shapes:
//
//	projects/{p}/locations/{l}/{collection}/{name}
//	projects/{p}/locations/{l}/certificateMaps/{map}/certificateMapEntries/{entry}
var CertificateManagerNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseCertificateManagerNativeID,
}

// certificateManagerLocation is where these resources live.
//
// Certificate maps, DNS authorizations and trust configs are global. Addressing
// them at the target's region answers "Malformed name: ... [Invalid location in
// resource URL path]", so the location is pinned rather than inherited - which
// also keeps create and discovery pointed at the same place, unlike a fixture
// that pins a region the discovery pass then fails to look in.
const certificateManagerLocation = "global"

// certificateManagerPathBuilder builds
// /projects/{p}/locations/global[/{parentType}/{parent}]/{resourceType}[/{name}].
// parentCollectionOf names the collection a nested type hangs off, for the one
// case where PathContext cannot say: a List with no parent.
var parentCollectionOf = map[string]string{
	"certificateMapEntries": "certificateMaps",
}

func certificateManagerPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s", ctx.Project, certificateManagerLocation)
	switch {
	case ctx.ParentType != "" && ctx.ParentResource != "":
		path += fmt.Sprintf("/%s/%s", ctx.ParentType, ctx.ParentResource)
	case parentCollectionOf[ctx.ResourceType] != "":
		// A List with no parent: discovery calls it with nothing to walk from,
		// and PathContext carries no ParentType then either, so the parent has
		// to come from the resource type itself. Without this the URL is
		// ".../locations/global/certificateMapEntries", which addresses a
		// collection that does not exist - the entries are never listed and the
		// resource never appears in inventory.
		//
		// Certificate Manager accepts "-" as a wildcard parent, enumerating
		// entries across every map in one call. That is cheaper than walking the
		// maps, which is what the same problem needed in Service Directory and
		// Cloud SQL, where no wildcard is offered.
		path += fmt.Sprintf("/%s/-", parentCollectionOf[ctx.ResourceType])
	}
	path += "/" + ctx.ResourceType
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// parseCertificateManagerNativeID restores the context a read needs, including
// the parent of a map entry - without it a read would address the
// location-level collection and 404.
func parseCertificateManagerNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" {
		return base.PathContext{}, fmt.Errorf("invalid certificate manager native ID: %s", nativeID)
	}
	ctx := base.PathContext{
		Project: parts[1],
		// Always "global" for this API; kept from the id so a native ID round
		// trips unchanged.
		Location:     parts[3],
		ResourceType: parts[4],
		ResourceName: parts[5],
	}
	switch len(parts) {
	case 6:
	case 8:
		ctx.ParentType = parts[4]
		ctx.ParentResource = parts[5]
		ctx.ResourceType = parts[6]
		ctx.ResourceName = parts[7]
	default:
		return base.PathContext{}, fmt.Errorf("invalid certificate manager native ID: %s", nativeID)
	}
	return ctx, nil
}

// extractOperationName returns the LRO name from a create or delete response.
func extractOperationName(response map[string]interface{}) string {
	if name, ok := response["name"].(string); ok && strings.Contains(name, "/operations/") {
		return name
	}
	return ""
}

// extractCertificateManagerNativeID builds the resource path. On an async
// create the response is an Operation rather than the resource, so the path
// comes from the context buildPathContext already filled in; a read or a list
// item reports its own full path.
func extractCertificateManagerNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name, ok := response["name"].(string); ok && !strings.Contains(name, "/operations/") {
		if i := strings.Index(name, "projects/"); i >= 0 {
			return name[i:]
		}
	}
	if ctx.ResourceName == "" {
		return ""
	}
	parent := ""
	if ctx.ParentType != "" && ctx.ParentResource != "" {
		parent = fmt.Sprintf("%s/%s/", ctx.ParentType, ctx.ParentResource)
	}
	return fmt.Sprintf("projects/%s/locations/%s/%s%s/%s",
		ctx.Project, certificateManagerLocation, parent, ctx.ResourceType, ctx.ResourceName)
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
