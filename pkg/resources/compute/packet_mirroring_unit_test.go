// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package compute

import (
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func mirroringProps() map[string]interface{} {
	return map[string]interface{}{
		"name":   "pm",
		"region": "europe-central2",
		"network": map[string]interface{}{
			"url": "https://www.googleapis.com/compute/v1/projects/p/global/networks/n",
		},
		"collectorIlb": map[string]interface{}{
			"url": "https://www.googleapis.com/compute/v1/projects/p/regions/europe-central2/forwardingRules/fr",
		},
		"mirroredResources": map[string]interface{}{
			"subnetworks": []interface{}{
				map[string]interface{}{
					"url": "https://www.googleapis.com/compute/v1/projects/p/regions/europe-central2/subnetworks/s",
				},
			},
		},
	}
}

// packetMirrorings.patch answers "Network cannot be changed" to any body whose
// network is not spelled exactly as stored, and a forma names it by self link,
// short path or bare name interchangeably. The field is createOnly, so the
// update body must not carry it at all.
func TestPacketMirroringUpdateDropsNetwork(t *testing.T) {
	out, err := packetMirroringRequestTransformer(mirroringProps(), base.TransformContext{
		Operation: resource.OperationUpdate,
	})
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	if _, ok := out["network"]; ok {
		t.Errorf("network must not reach a patch body: %#v", out)
	}
	if _, ok := out["collectorIlb"]; !ok {
		t.Errorf("collectorIlb is mutable and must survive: %#v", out)
	}
}

// Create needs the network - it is required there and the resource cannot be
// built without it.
func TestPacketMirroringCreateKeepsNetwork(t *testing.T) {
	out, err := packetMirroringRequestTransformer(mirroringProps(), base.TransformContext{
		Operation: resource.OperationCreate,
	})
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	if _, ok := out["network"]; !ok {
		t.Errorf("network is required on insert: %#v", out)
	}
	// Nothing is padded on create either: an empty selector list would say
	// nothing the body does not already say.
	mirrored, ok := out["mirroredResources"].(map[string]interface{})
	if !ok {
		t.Fatalf("mirroredResources missing: %#v", out)
	}
	if _, ok := mirrored["tags"]; ok {
		t.Errorf("create bodies must pass through untouched: %#v", mirrored)
	}
}

// patch is a JSON merge patch: a selector left out of mirroredResources keeps
// its old value. A forma that drops every tag would leave the tags mirroring in
// place, so the absent selectors go out as explicit empty lists.
func TestPacketMirroringUpdateClearsAbsentSelectors(t *testing.T) {
	out, err := packetMirroringRequestTransformer(mirroringProps(), base.TransformContext{
		Operation: resource.OperationUpdate,
	})
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	mirrored, ok := out["mirroredResources"].(map[string]interface{})
	if !ok {
		t.Fatalf("mirroredResources missing: %#v", out)
	}
	for _, selector := range []string{"instances", "tags"} {
		got, present := mirrored[selector]
		if !present {
			t.Errorf("%q must be sent as an empty list so the patch clears it: %#v", selector, mirrored)
			continue
		}
		if list, ok := got.([]interface{}); !ok || len(list) != 0 {
			t.Errorf("%q must be an empty list, got %#v", selector, got)
		}
	}
	subnets, ok := mirrored["subnetworks"].([]interface{})
	if !ok || len(subnets) != 1 {
		t.Errorf("a declared selector must be sent as declared: %#v", mirrored["subnetworks"])
	}
}

// A resource with no mirroredResources at all is left alone rather than given an
// empty selector object the API would reject.
func TestPacketMirroringUpdateWithoutSelectorsIsUntouched(t *testing.T) {
	out, err := packetMirroringRequestTransformer(map[string]interface{}{
		"name":        "pm",
		"description": "d",
	}, base.TransformContext{Operation: resource.OperationUpdate})
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	if _, ok := out["mirroredResources"]; ok {
		t.Errorf("nothing should be invented: %#v", out)
	}
}

// The transformer must not write through to the caller's map: base builds the
// path context from the raw properties and other code reads them afterwards.
func TestPacketMirroringRequestDoesNotMutateInput(t *testing.T) {
	in := mirroringProps()
	if _, err := packetMirroringRequestTransformer(in, base.TransformContext{
		Operation: resource.OperationUpdate,
	}); err != nil {
		t.Fatalf("transform: %v", err)
	}
	if _, ok := in["network"]; !ok {
		t.Errorf("input map was mutated: %#v", in)
	}
	mirrored := in["mirroredResources"].(map[string]interface{})
	if len(mirrored) != 1 {
		t.Errorf("input selectors were mutated: %#v", mirrored)
	}
}

// Every reference comes back with a second spelling by numeric id. It is
// output-only and lives inside a sub-resource, where no schema hint reaches it,
// so it must be stripped or every read plans an update that changes nothing.
func TestPacketMirroringResponseStripsCanonicalURLs(t *testing.T) {
	out := packetMirroringResponseTransformer(map[string]interface{}{
		"region": "https://www.googleapis.com/compute/v1/projects/p/regions/europe-central2",
		"network": map[string]interface{}{
			"url":          "https://www.googleapis.com/compute/v1/projects/p/global/networks/n",
			"canonicalUrl": "https://www.googleapis.com/compute/v1/projects/p/global/networks/322",
		},
		"collectorIlb": map[string]interface{}{
			"url":          "https://www.googleapis.com/compute/v1/projects/p/regions/europe-central2/forwardingRules/fr",
			"canonicalUrl": "https://www.googleapis.com/compute/v1/projects/p/regions/europe-central2/forwardingRules/649",
		},
		"mirroredResources": map[string]interface{}{
			"subnetworks": []interface{}{
				map[string]interface{}{
					"url":          "https://www.googleapis.com/compute/v1/projects/p/regions/europe-central2/subnetworks/s",
					"canonicalUrl": "https://www.googleapis.com/compute/v1/projects/p/regions/europe-central2/subnetworks/870",
				},
			},
			"instances": []interface{}{
				map[string]interface{}{
					"url":          "https://www.googleapis.com/compute/v1/projects/p/zones/z/instances/i",
					"canonicalUrl": "https://www.googleapis.com/compute/v1/projects/p/zones/z/instances/111",
				},
			},
			"tags": []interface{}{"t"},
		},
	}, base.TransformContext{})

	network := out["network"].(map[string]interface{})
	if _, ok := network["canonicalUrl"]; ok {
		t.Errorf("network.canonicalUrl must be stripped: %#v", network)
	}
	if network["url"] != "https://www.googleapis.com/compute/v1/projects/p/global/networks/n" {
		t.Errorf("network.url must survive: %#v", network)
	}
	if _, ok := out["collectorIlb"].(map[string]interface{})["canonicalUrl"]; ok {
		t.Errorf("collectorIlb.canonicalUrl must be stripped: %#v", out["collectorIlb"])
	}

	mirrored := out["mirroredResources"].(map[string]interface{})
	for _, selector := range []string{"subnetworks", "instances"} {
		entry := mirrored[selector].([]interface{})[0].(map[string]interface{})
		if _, ok := entry["canonicalUrl"]; ok {
			t.Errorf("%s[0].canonicalUrl must be stripped: %#v", selector, entry)
		}
		if entry["url"] == "" {
			t.Errorf("%s[0].url must survive: %#v", selector, entry)
		}
	}
	if !reflect.DeepEqual(mirrored["tags"], []interface{}{"t"}) {
		t.Errorf("tags are plain strings and must pass through: %#v", mirrored["tags"])
	}

	if out["region"] != "europe-central2" {
		t.Errorf("region must be the short name, got %v", out["region"])
	}
}

// A read of a resource that names nothing but tags must not gain empty
// reference lists, and a response without references at all must not panic.
func TestPacketMirroringResponseToleratesMissingReferences(t *testing.T) {
	out := packetMirroringResponseTransformer(map[string]interface{}{
		"name": "pm",
		"mirroredResources": map[string]interface{}{
			"tags": []interface{}{"t"},
		},
	}, base.TransformContext{})

	mirrored := out["mirroredResources"].(map[string]interface{})
	if len(mirrored) != 1 {
		t.Errorf("nothing should be invented on read: %#v", mirrored)
	}
	if _, ok := out["network"]; ok {
		t.Errorf("network must not be invented: %#v", out)
	}
}
