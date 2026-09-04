// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package networkconnectivity

import (
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestExpandHubRef(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		project string
		want    string
	}{
		{
			name:    "short hub id is expanded to a full path",
			value:   "my-hub",
			project: "proj-1",
			want:    "projects/proj-1/locations/global/hubs/my-hub",
		},
		{
			name:    "a value that is already a path is left alone",
			value:   "projects/other/locations/global/hubs/shared",
			project: "proj-1",
			want:    "projects/other/locations/global/hubs/shared",
		},
		{
			name:    "empty stays empty rather than becoming a path to nothing",
			value:   "",
			project: "proj-1",
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expandHubRef(tt.value, tt.project); got != tt.want {
				t.Errorf("expandHubRef() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShortenHubRef(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "full path reduces to the short id",
			value: "projects/proj-1/locations/global/hubs/my-hub",
			want:  "my-hub",
		},
		{
			name:  "an already-short id is unchanged, as a read may report it",
			value: "my-hub",
			want:  "my-hub",
		},
		{
			name:  "empty stays empty",
			value: "",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shortenHubRef(tt.value); got != tt.want {
				t.Errorf("shortenHubRef() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Expanding on the way out and shortening on the way back must be exact
// inverses. `hub` is immutable, so if they are not, the declared value and the
// stored state disagree forever and every re-apply plans a replacement the API
// then refuses.
func TestHubRefRoundTrips(t *testing.T) {
	for _, short := range []string{"my-hub", "h", "hub-with-dashes-123", "formae-test-nc-spk-hub-abc123"} {
		full := expandHubRef(short, "proj-1")
		if got := shortenHubRef(full); got != short {
			t.Errorf("round trip of %q via %q gave %q", short, full, got)
		}
	}
}

func TestSpokeRequestTransformer(t *testing.T) {
	tests := []struct {
		name  string
		props map[string]interface{}
		op    resource.Operation
		want  map[string]interface{}
	}{
		{
			name: "create expands the hub and keeps everything else",
			props: map[string]interface{}{
				"name":        "my-spoke",
				"hub":         "my-hub",
				"description": "d",
				"network":     "https://www.googleapis.com/compute/v1/projects/p/global/networks/n",
			},
			op: resource.OperationCreate,
			want: map[string]interface{}{
				"name":        "my-spoke",
				"hub":         "projects/p/locations/global/hubs/my-hub",
				"description": "d",
				"linkedVpcNetwork": map[string]interface{}{
					"uri": "https://www.googleapis.com/compute/v1/projects/p/global/networks/n",
				},
			},
		},
		{
			name: "update carries only what a patch may change",
			props: map[string]interface{}{
				"name":        "my-spoke",
				"hub":         "my-hub",
				"description": "d",
				"labels":      map[string]interface{}{"k": "v"},
				"network":     "https://www.googleapis.com/compute/v1/projects/p/global/networks/n",
				"group":       "projects/p/locations/global/hubs/my-hub/groups/default",
				"spokeType":   "VPC_NETWORK",
				"state":       "ACTIVE",
				"uniqueId":    "349fcee1",
				"etag":        "2",
			},
			op: resource.OperationUpdate,
			want: map[string]interface{}{
				"description": "d",
				"labels":      map[string]interface{}{"k": "v"},
			},
		},
		{
			name: "a hub already given as a full path is not expanded twice",
			props: map[string]interface{}{
				"name": "my-spoke",
				"hub":  "projects/other/locations/global/hubs/shared",
			},
			op: resource.OperationCreate,
			want: map[string]interface{}{
				"name": "my-spoke",
				"hub":  "projects/other/locations/global/hubs/shared",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := spokeRequestTransformer(
				tt.props, base.TransformContext{Project: "p", Operation: tt.op})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %#v\nwant %#v", got, tt.want)
			}
		})
	}
}

// The caller's map must survive a transform untouched: base builds the path
// context from the raw properties, and a create that mutated them in place
// would corrupt the id the URL is built from.
func TestSpokeRequestTransformerDoesNotMutateInput(t *testing.T) {
	props := map[string]interface{}{"name": "my-spoke", "hub": "my-hub"}
	if _, err := spokeRequestTransformer(
		props, base.TransformContext{Project: "p", Operation: resource.OperationCreate}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if props["hub"] != "my-hub" {
		t.Errorf("input was mutated: hub = %q", props["hub"])
	}
}

func TestSpokeResponseTransformer(t *testing.T) {
	got := spokeResponseTransformer(map[string]interface{}{
		"name":      "projects/p/locations/global/spokes/my-spoke",
		"hub":       "projects/p/locations/global/hubs/my-hub",
		"spokeType": "VPC_NETWORK",
		"state":     "ACTIVE",
		"uniqueId":  "349fcee1",
		"group":     "projects/p/locations/global/hubs/my-hub/groups/default",
		"etag":      "2",
		"linkedVpcNetwork": map[string]interface{}{
			"uri":                         "https://www.googleapis.com/compute/v1/projects/p/global/networks/n",
			"vpcNetwork":                  "projects/p/global/networks/n",
			"proposedIncludeExportRanges": []interface{}{"10.0.0.0/8"},
			"proposedExcludeExportRanges": []interface{}{"10.1.0.0/16"},
			"producerVpcSpokes":           []interface{}{"projects/p/locations/global/spokes/other"},
		},
	}, base.TransformContext{Project: "p"})

	want := map[string]interface{}{
		"name":      "my-spoke",
		"hub":       "my-hub",
		"spokeType": "VPC_NETWORK",
		"state":     "ACTIVE",
		"uniqueId":  "349fcee1",
		"group":     "projects/p/locations/global/hubs/my-hub/groups/default",
		"etag":      "2",
		"network":   "https://www.googleapis.com/compute/v1/projects/p/global/networks/n",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
}

// The declared members of linkedVpcNetwork must survive the strip and land on
// the flat properties the schema declares, or a forma that restricts
// propagation would see its ranges vanish and drift forever.
func TestSpokeResponseTransformerKeepsDeclaredLinkedMembers(t *testing.T) {
	got := spokeResponseTransformer(map[string]interface{}{
		"name": "projects/p/locations/global/spokes/my-spoke",
		"linkedVpcNetwork": map[string]interface{}{
			"uri":                 "https://www.googleapis.com/compute/v1/projects/p/global/networks/n",
			"includeExportRanges": []interface{}{"10.0.0.0/8"},
			"excludeExportRanges": []interface{}{"10.1.0.0/16"},
			"vpcNetwork":          "projects/p/global/networks/n",
		},
	}, base.TransformContext{Project: "p"})

	want := map[string]interface{}{
		"network":             "https://www.googleapis.com/compute/v1/projects/p/global/networks/n",
		"includeExportRanges": []interface{}{"10.0.0.0/8"},
		"excludeExportRanges": []interface{}{"10.1.0.0/16"},
	}
	for k, v := range want {
		if !reflect.DeepEqual(got[k], v) {
			t.Errorf("%s: got %#v, want %#v", k, got[k], v)
		}
	}
	if _, nested := got["linkedVpcNetwork"]; nested {
		t.Error("linkedVpcNetwork must be unpacked onto the flat properties")
	}
}

// A response that reports the hub as a bare short id already - or omits it -
// must not be rewritten into something else.
func TestSpokeResponseTransformerToleratesShortHub(t *testing.T) {
	got := spokeResponseTransformer(map[string]interface{}{
		"name": "projects/p/locations/global/spokes/my-spoke",
		"hub":  "my-hub",
	}, base.TransformContext{Project: "p"})
	if got["hub"] != "my-hub" {
		t.Errorf("hub = %#v, want %q", got["hub"], "my-hub")
	}

	got = spokeResponseTransformer(map[string]interface{}{
		"name": "projects/p/locations/global/spokes/my-spoke",
	}, base.TransformContext{Project: "p"})
	if _, present := got["hub"]; present {
		t.Errorf("an absent hub was materialised: %#v", got)
	}
}

// A spoke must address locations/global whatever region the target carries -
// the discovery document's placement under projects.locations notwithstanding.
func TestSpokePathIsAlwaysGlobal(t *testing.T) {
	path := networkConnectivityPathBuilder(base.PathContext{
		Project:      "p",
		Location:     "europe-west1",
		ResourceType: "spokes",
		ResourceName: "my-spoke",
	})
	want := "/projects/p/locations/global/spokes/my-spoke"
	if path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
}

// The schema is flat and the wire is nested, so the two transformers have to be
// exact inverses: whatever a forma declares must come back identical after a
// create assembles it and a read unpacks it. If they drift, the declared value
// and stored state disagree on a createOnly field and every re-apply plans a
// replacement that the API then refuses.
func TestSpokeLinkedVpcNetworkFlattensAndAssembles(t *testing.T) {
	const uri = "https://www.googleapis.com/compute/v1/projects/p/global/networks/n"
	declared := map[string]interface{}{
		"name":                "spk",
		"hub":                 "my-hub",
		"network":             uri,
		"includeExportRanges": []interface{}{"10.0.0.0/8"},
	}

	sent, err := spokeRequestTransformer(declared, base.TransformContext{Project: "p"})
	if err != nil {
		t.Fatalf("request transform: %v", err)
	}
	linked, ok := sent["linkedVpcNetwork"].(map[string]interface{})
	if !ok {
		t.Fatalf("linkedVpcNetwork not assembled, got %#v", sent)
	}
	if linked["uri"] != uri {
		t.Errorf("uri = %v, want %v", linked["uri"], uri)
	}
	if _, stillFlat := sent["network"]; stillFlat {
		t.Error("flat network must not also be sent to the API")
	}

	// The API echoes the object back, with its output-only members attached.
	linked["vpcNetwork"] = "projects/p/global/networks/n"
	linked["proposedIncludeExportRanges"] = []interface{}{"x"}
	got := spokeResponseTransformer(
		map[string]interface{}{"name": "projects/p/locations/global/spokes/spk", "linkedVpcNetwork": linked},
		base.TransformContext{Project: "p"})

	if got["network"] != uri {
		t.Errorf("network = %v, want %v", got["network"], uri)
	}
	if _, nested := got["linkedVpcNetwork"]; nested {
		t.Error("linkedVpcNetwork must be unpacked, not left in state")
	}
	for _, leaked := range []string{"vpcNetwork", "proposedIncludeExportRanges"} {
		if _, present := got[leaked]; present {
			t.Errorf("output-only %q leaked into state", leaked)
		}
	}
}
