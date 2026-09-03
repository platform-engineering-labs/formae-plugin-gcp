// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package analyticshub

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestPathBuilder(t *testing.T) {
	tests := []struct {
		name string
		ctx  base.PathContext
		want string
	}{
		{
			name: "top-level collection",
			ctx:  base.PathContext{Project: "p", Location: "eu", ResourceType: "dataExchanges"},
			want: "/projects/p/locations/eu/dataExchanges",
		},
		{
			name: "top-level resource",
			ctx:  base.PathContext{Project: "p", Location: "eu", ResourceType: "dataExchanges", ResourceName: "de1"},
			want: "/projects/p/locations/eu/dataExchanges/de1",
		},
		{
			name: "nested resource",
			ctx: base.PathContext{
				Project: "p", Location: "eu",
				ParentType: "dataExchanges", ParentResource: "de1",
				ResourceType: "listings", ResourceName: "li1",
			},
			want: "/projects/p/locations/eu/dataExchanges/de1/listings/li1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := analyticsHubPathBuilder(tt.ctx); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseNativeID(t *testing.T) {
	ctx, err := parseAnalyticsHubNativeID("projects/p/locations/eu/dataExchanges/de1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.ResourceType != "dataExchanges" || ctx.ResourceName != "de1" || ctx.Location != "eu" {
		t.Errorf("top-level parse wrong: %+v", ctx)
	}

	ctx, err = parseAnalyticsHubNativeID("projects/p/locations/eu/dataExchanges/de1/listings/li1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.ParentType != "dataExchanges" || ctx.ParentResource != "de1" ||
		ctx.ResourceType != "listings" || ctx.ResourceName != "li1" {
		t.Errorf("nested parse wrong: %+v", ctx)
	}

	if _, err := parseAnalyticsHubNativeID("projects/p/locations/eu/dataExchanges"); err == nil {
		t.Error("a 5-segment id is not a resource and must be rejected")
	}
}

// The API reports "name" as a full path and never reports "location" or
// "dataExchange" as fields, but a forma declares all three.
func TestShortNameWithLocation(t *testing.T) {
	out := shortNameWithLocation("listings")(map[string]interface{}{
		"name": "projects/p/locations/eu/dataExchanges/de1/listings/li1",
	}, base.TransformContext{})
	if out["name"] != "li1" || out["location"] != "eu" || out["dataExchange"] != "de1" {
		t.Errorf("nested: %+v", out)
	}

	// An exchange's path must not be read as a listing's.
	out = shortNameWithLocation("listings")(map[string]interface{}{
		"name": "projects/p/locations/eu/dataExchanges/de1",
	}, base.TransformContext{})
	if out["name"] != "projects/p/locations/eu/dataExchanges/de1" {
		t.Errorf("foreign collection must be left alone: %+v", out)
	}
}

// A forma passes a bare dataset id via a resolvable; the API wants a full path.
// Expand on write, shorten on read - otherwise every comparison reports drift.
func TestListingDatasetRoundTrip(t *testing.T) {
	body, err := listingRequest(map[string]interface{}{
		"name":         "li1",
		"location":     "eu",
		"dataExchange": "de1",
		"displayName":  "L",
		"bigqueryDataset": map[string]interface{}{
			"dataset": "my_ds",
		},
	}, base.TransformContext{Project: "p"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, k := range []string{"location", "dataExchange"} {
		if _, ok := body[k]; ok {
			t.Errorf("%q addresses the resource in the URL and must not be a body field", k)
		}
	}
	if _, ok := body["name"]; !ok {
		t.Error("name must survive: base.Create reads the create id out of it")
	}
	src := body["bigqueryDataset"].(map[string]interface{})
	if got, want := src["dataset"], "projects/p/datasets/my_ds"; got != want {
		t.Errorf("dataset = %v, want %v", got, want)
	}

	out := listingResponse(map[string]interface{}{
		"name":            "projects/p/locations/eu/dataExchanges/de1/listings/li1",
		"bigqueryDataset": map[string]interface{}{"dataset": "projects/p/datasets/my_ds"},
	}, base.TransformContext{})
	if got := out["bigqueryDataset"].(map[string]interface{})["dataset"]; got != "my_ds" {
		t.Errorf("dataset = %v, want my_ds", got)
	}
}

// A full path written by hand must not be expanded a second time.
func TestListingDatasetLeavesFullPathAlone(t *testing.T) {
	body, err := listingRequest(map[string]interface{}{
		"bigqueryDataset": map[string]interface{}{"dataset": "projects/other/datasets/d"},
	}, base.TransformContext{Project: "p"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	src := body["bigqueryDataset"].(map[string]interface{})
	if got, want := src["dataset"], "projects/other/datasets/d"; got != want {
		t.Errorf("dataset = %v, want %v", got, want)
	}
}

// The published dataset is fixed at creation and the update mask is built from
// the body, so an update that leaves bigqueryDataset in it is refused outright.
func TestListingUpdateDropsPublishedDataset(t *testing.T) {
	body, err := listingRequest(map[string]interface{}{
		"name":            "li1",
		"project":         "p",
		"displayName":     "L",
		"description":     "changed",
		"bigqueryDataset": map[string]interface{}{"dataset": "my_ds"},
	}, base.TransformContext{Project: "p", Operation: resource.OperationUpdate})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, k := range []string{"bigqueryDataset", "name", "project"} {
		if _, ok := body[k]; ok {
			t.Errorf("%q must not reach the update mask", k)
		}
	}
	if body["description"] != "changed" {
		t.Errorf("the field actually being changed must survive: %+v", body)
	}
}

// The API populates the replication state and an all-false export policy on
// every listing. Neither is declarable, and an unexpected key under a declared
// property reads back as drift.
func TestListingResponseDropsProviderNoise(t *testing.T) {
	out := listingResponse(map[string]interface{}{
		"name": "projects/p/locations/eu/dataExchanges/de1/listings/li1",
		"bigqueryDataset": map[string]interface{}{
			"dataset":                "projects/p/datasets/my_ds",
			"effectiveReplicas":      []interface{}{map[string]interface{}{"location": "eu"}},
			"restrictedExportPolicy": map[string]interface{}{"enabled": false},
		},
	}, base.TransformContext{})
	src := out["bigqueryDataset"].(map[string]interface{})
	for _, k := range []string{"effectiveReplicas", "restrictedExportPolicy"} {
		if _, ok := src[k]; ok {
			t.Errorf("%q is provider noise and must be dropped", k)
		}
	}
	if src["dataset"] != "my_ds" {
		t.Errorf("dataset = %v, want my_ds", src["dataset"])
	}
}
