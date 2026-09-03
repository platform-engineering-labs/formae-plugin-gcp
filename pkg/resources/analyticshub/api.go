// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package analyticshub

import (
	"fmt"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// AnalyticsHubAPI - Analytics Hub API v1. Everything is location-scoped, and
// listings and query templates are nested under a data exchange. Unlike most
// GCP APIs at this shape, create and delete are synchronous: create answers
// with the resource, delete with an empty body. There is no operations
// collection to poll.
var AnalyticsHubAPI = base.APIConfig{
	BaseURL:     "https://analyticshub.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: analyticsHubPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

var AnalyticsHubOperations = base.OperationConfig{
	Synchronous:       true,
	NativeIDExtractor: extractAnalyticsHubNativeID,
}

// AnalyticsHubNativeID - the full resource path, e.g.
// "projects/{p}/locations/{l}/dataExchanges/{de}/listings/{name}".
var AnalyticsHubNativeID = base.NativeIDConfig{
	Format: base.FullPathFormat,
	Parser: parseAnalyticsHubNativeID,
}

// analyticsHubPathBuilder builds
//
//	/projects/{p}/locations/{l}/dataExchanges[/{name}]
//	/projects/{p}/locations/{l}/dataExchanges/{parent}/{resourceType}[/{name}]
//
// A nested collection has no listable URL of its own - there is no
// "listings" collection spanning exchanges - so discovery walks the
// exchanges. See exchange_walking_list.go.
func analyticsHubPathBuilder(ctx base.PathContext) string {
	path := fmt.Sprintf("/projects/%s/locations/%s", ctx.Project, ctx.Location)
	if ctx.ParentType != "" && ctx.ParentResource != "" {
		path = fmt.Sprintf("%s/%s/%s", path, ctx.ParentType, ctx.ParentResource)
	}
	path += "/" + ctx.ResourceType
	if ctx.ResourceName != "" {
		path += "/" + ctx.ResourceName
	}
	return path
}

// extractAnalyticsHubNativeID reads the full path the API reports. Create is
// synchronous, so the response is always the resource itself.
func extractAnalyticsHubNativeID(response map[string]interface{}, ctx base.PathContext) string {
	if name, ok := response["name"].(string); ok && name != "" {
		if i := strings.Index(name, "projects/"); i >= 0 {
			return name[i:]
		}
	}
	// Fall back to context, which buildPathContext has already filled in from
	// the declared id.
	path := fmt.Sprintf("projects/%s/locations/%s", ctx.Project, ctx.Location)
	if ctx.ParentType != "" && ctx.ParentResource != "" {
		path = fmt.Sprintf("%s/%s/%s", path, ctx.ParentType, ctx.ParentResource)
	}
	return fmt.Sprintf("%s/%s/%s", path, ctx.ResourceType, ctx.ResourceName)
}

// parseAnalyticsHubNativeID handles both the top-level form
// (projects/{p}/locations/{l}/dataExchanges/{name}, 6 segments) and the nested
// form (.../dataExchanges/{parent}/{resourceType}/{name}, 8 segments).
func parseAnalyticsHubNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" {
		return base.PathContext{}, fmt.Errorf("invalid Analytics Hub native ID: %s", nativeID)
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
			"invalid Analytics Hub native ID: %s (expected 6 or 8 path segments, got %d)", nativeID, len(parts))
	}
}

// shortNameWithLocation puts back the short name and the location. The API
// reports "name" as a full path and never reports "location" as a field of its
// own, but a forma declares it - and a declared field the read never reports
// would look like it went missing on every comparison.
//
// The collection is passed in so a path is only parsed when it is the expected
// kind: a data exchange's path must not be read as a listing's.
func shortNameWithLocation(collection string) base.ResponseTransformerFunc {
	return func(props map[string]interface{}, _ base.TransformContext) map[string]interface{} {
		out := make(map[string]interface{}, len(props)+2)
		for k, v := range props {
			out[k] = v
		}
		name, ok := props["name"].(string)
		if !ok {
			return out
		}
		parts := strings.Split(name, "/")
		// Top-level: projects/{p}/locations/{l}/{collection}/{name}
		if len(parts) == 6 && parts[2] == "locations" && parts[4] == collection {
			out["location"] = parts[3]
			out["name"] = parts[5]
			return out
		}
		// Nested: .../dataExchanges/{parent}/{collection}/{name}
		if len(parts) == 8 && parts[2] == "locations" && parts[6] == collection {
			out["location"] = parts[3]
			out["dataExchange"] = parts[5]
			out["name"] = parts[7]
		}
		return out
	}
}

// dropPathFields removes the fields that address the resource in the URL and
// are not body fields. "name" stays: base.Create reads the create id
// (?dataExchangeId=, ?listingId=, ?queryTemplateId=) out of it and deletes it
// from the body itself.
func dropPathFields(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(props))
	for k, v := range props {
		switch k {
		case "location", "dataExchange":
			continue
		case "name", "project":
			// The update mask is built from the body, so anything left in it is
			// something the API is asked to change. The id and the project
			// address the resource rather than describing it, and a mask naming
			// either is refused - which failed the update on all three types
			// while the create was fine.
			if ctx.Operation == resource.OperationUpdate {
				continue
			}
		}
		body[k] = v
	}
	return body, nil
}

// listingRequest expands the published dataset into the full path the API
// wants. A forma passes `dataset.res.datasetId` - a resolvable, so formae
// creates the dataset first and the listing gets the ordering edge - which
// resolves to the bare dataset id.
func listingRequest(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	body, err := dropPathFields(props, ctx)
	if err != nil {
		return nil, err
	}
	// The published dataset is fixed at creation ("The field 'bigquery_dataset'
	// cannot be updated on this listing"), and the update mask is built from
	// the body, so leaving it in asks the API to change it and fails every
	// update - even one that only touches the description.
	if ctx.Operation == resource.OperationUpdate {
		delete(body, "bigqueryDataset")
		return body, nil
	}
	source, ok := body["bigqueryDataset"].(map[string]interface{})
	if !ok {
		return body, nil
	}
	copied := make(map[string]interface{}, len(source))
	for k, v := range source {
		copied[k] = v
	}
	if dataset, ok := copied["dataset"].(string); ok && dataset != "" && !strings.Contains(dataset, "/") {
		copied["dataset"] = fmt.Sprintf("projects/%s/datasets/%s", ctx.Project, dataset)
	}
	body["bigqueryDataset"] = copied
	return body, nil
}

// listingResponse is the mirror of listingRequest. Without the symmetry the
// declared dataset id could never equal the full path read back, and every
// comparison step would report drift on a listing that is in fact correct.
func listingResponse(props map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	out := shortNameWithLocation("listings")(props, ctx)
	source, ok := out["bigqueryDataset"].(map[string]interface{})
	if !ok {
		return out
	}
	copied := make(map[string]interface{}, len(source))
	for k, v := range source {
		copied[k] = v
	}
	if dataset, ok := copied["dataset"].(string); ok {
		if i := strings.LastIndex(dataset, "/datasets/"); i >= 0 {
			copied["dataset"] = dataset[i+len("/datasets/"):]
		}
	}
	// Provider-assigned noise inside a declared object: the API reports the
	// replication state of the shared dataset and an all-false export policy on
	// every listing, neither of which a forma declares. Unexpected keys under a
	// declared property read back as drift, so they are dropped the way
	// separately-owned mirrors are elsewhere.
	for _, k := range []string{"effectiveReplicas", "restrictedExportPolicy"} {
		delete(copied, k)
	}
	out["bigqueryDataset"] = copied
	return out
}
