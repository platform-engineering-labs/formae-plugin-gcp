// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package networksecurity

import (
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// Every collection added in this batch is global except the two Secure Web
// Proxy ones. Both answers were established against the live API: a global
// collection asked for a region, and a regional collection asked for global,
// each answer 400 rather than returning nothing.
func TestLocationOfBatchCollections(t *testing.T) {
	tests := []struct {
		resourceType string
		location     string
		want         string
	}{
		{"clientTlsPolicies", "europe-central2", "global"},
		{"serverTlsPolicies", "europe-central2", "global"},
		{"backendAuthenticationConfigs", "europe-central2", "global"},
		{"authorizationPolicies", "europe-central2", "global"},
		{"dnsThreatDetectors", "europe-central2", "global"},
		{"gatewaySecurityPolicies", "europe-central2", "europe-central2"},
		{"rules", "europe-central2", "europe-central2"},
	}
	for _, tt := range tests {
		t.Run(tt.resourceType+"/"+tt.location, func(t *testing.T) {
			got := locationOf(base.PathContext{ResourceType: tt.resourceType, Location: tt.location})
			if got != tt.want {
				t.Errorf("locationOf(%s, %q) = %q, want %q", tt.resourceType, tt.location, got, tt.want)
			}
		})
	}
}

func TestNetworkSecurityPathBuilder(t *testing.T) {
	tests := []struct {
		name string
		ctx  base.PathContext
		want string
	}{
		{
			name: "a global collection ignores the location it was handed",
			ctx: base.PathContext{
				Project: "p", ResourceType: "clientTlsPolicies", Location: "europe-central2",
			},
			want: "/projects/p/locations/global/clientTlsPolicies",
		},
		{
			name: "a regional resource keeps its region",
			ctx: base.PathContext{
				Project: "p", ResourceType: "gatewaySecurityPolicies",
				ResourceName: "pol", Location: "europe-central2",
			},
			want: "/projects/p/locations/europe-central2/gatewaySecurityPolicies/pol",
		},
		{
			name: "a nested rule renders its parent segment",
			ctx: base.PathContext{
				Project: "p", ResourceType: "rules", ResourceName: "r1",
				ParentType: "gatewaySecurityPolicies", ParentResource: "pol",
				Location: "europe-central2",
			},
			want: "/projects/p/locations/europe-central2/gatewaySecurityPolicies/pol/rules/r1",
		},
		{
			name: "a rule create posts to the collection under its parent",
			ctx: base.PathContext{
				Project: "p", ResourceType: "rules",
				ParentType: "gatewaySecurityPolicies", ParentResource: "pol",
				Location: "europe-central2",
			},
			want: "/projects/p/locations/europe-central2/gatewaySecurityPolicies/pol/rules",
		},
		{
			// Discovery lists with no parent at all. Without the wildcard the
			// URL addresses a collection that does not exist and no rule is
			// ever discovered.
			name: "a parentless list falls back to the wildcard parent",
			ctx: base.PathContext{
				Project: "p", ResourceType: "rules", Location: "europe-central2", IsList: true,
			},
			want: "/projects/p/locations/europe-central2/gatewaySecurityPolicies/-/rules",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := networkSecurityPathBuilder(tt.ctx); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestParseNetworkSecurityNativeID(t *testing.T) {
	tests := []struct {
		name     string
		nativeID string
		want     base.PathContext
		wantErr  bool
	}{
		{
			name:     "a flat resource",
			nativeID: "projects/p/locations/global/clientTlsPolicies/ctp",
			want: base.PathContext{
				Project: "p", Location: "global",
				ResourceType: "clientTlsPolicies", ResourceName: "ctp",
			},
		},
		{
			name:     "a nested rule keeps its parent",
			nativeID: "projects/p/locations/europe-central2/gatewaySecurityPolicies/pol/rules/r1",
			want: base.PathContext{
				Project: "p", Location: "europe-central2",
				ParentType: "gatewaySecurityPolicies", ParentResource: "pol",
				ResourceType: "rules", ResourceName: "r1",
			},
		},
		{
			name:     "something that is not a resource path is rejected",
			nativeID: "clientTlsPolicies/ctp",
			wantErr:  true,
		},
		{
			name:     "an unexpected depth is rejected rather than half-parsed",
			nativeID: "projects/p/locations/global/a/b/c/d/e/f",
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseNetworkSecurityNativeID(tt.nativeID)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %#v\nwant %#v", got, tt.want)
			}
		})
	}
}

// A native ID has to survive the round trip through the parser and the path
// builder, or a read addresses something other than what a create built.
func TestNestedRuleNativeIDRoundTrips(t *testing.T) {
	const nativeID = "projects/p/locations/europe-central2/gatewaySecurityPolicies/pol/rules/r1"

	ctx, err := parseNetworkSecurityNativeID(nativeID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := networkSecurityPathBuilder(ctx); got != "/"+nativeID {
		t.Errorf("path builder gave %q, want %q", got, "/"+nativeID)
	}
	if got := extractNetworkSecurityNativeID(map[string]interface{}{}, ctx); got != nativeID {
		t.Errorf("native ID extractor gave %q, want %q", got, nativeID)
	}
}

// On a create the response is an Operation, so the id comes from the context;
// on a list item there is no context to speak of and the item's own path is the
// only source. Both paths have to produce the parent segments.
func TestExtractNetworkSecurityNativeIDFromListItem(t *testing.T) {
	item := map[string]interface{}{
		"name": "projects/p/locations/europe-central2/gatewaySecurityPolicies/pol/rules/r1",
	}
	want := "projects/p/locations/europe-central2/gatewaySecurityPolicies/pol/rules/r1"
	if got := extractNetworkSecurityNativeID(item, base.PathContext{Project: "p", ResourceType: "rules"}); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// dnsThreatDetectors is the one synchronous collection: its POST answers with
// the resource, so extractOperationName finds no operation. The guard against
// that is the per-resource OperationConfig, not the extractor - this pins the
// premise.
func TestExtractOperationNameIgnoresAResourceResponse(t *testing.T) {
	resourceResponse := map[string]interface{}{
		"name":     "projects/p/locations/global/dnsThreatDetectors/dtd",
		"provider": "INFOBLOX",
	}
	if got := extractOperationName(resourceResponse); got != "" {
		t.Errorf("extractOperationName() = %q, want an empty string", got)
	}
	if !NetworkSecuritySyncOperations.Synchronous {
		t.Error("dnsThreatDetectors must be registered with the synchronous operation config")
	}
	if NetworkSecuritySyncOperations.OperationIDExtractor == nil {
		t.Error("the sync config needs a non-nil OperationIDExtractor or the registry replaces it wholesale")
	}
}
