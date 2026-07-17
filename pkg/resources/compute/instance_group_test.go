// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

const (
	vmA = "https://www.googleapis.com/compute/v1/projects/p/zones/z/instances/a"
	vmB = "https://www.googleapis.com/compute/v1/projects/p/zones/z/instances/b"
	vmC = "https://www.googleapis.com/compute/v1/projects/p/zones/z/instances/c"
)

// listInstances returns members under items[].instance as full self-link URLs;
// the read path surfaces them sorted so membership round-trips deterministically
// against a resolvable reference (agentVm.res.selfLink resolves to the same URL).
func TestMembersFromListInstancesResponse(t *testing.T) {
	body := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"instance": vmB, "status": "RUNNING"},
			map[string]interface{}{"instance": vmA, "status": "RUNNING"},
		},
	}
	got := membersFromListInstancesResponse(body)
	want := []string{vmA, vmB} // sorted
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// An empty group (no items key) yields no members, so the read omits `instances`
// entirely — matching an absent desired field rather than drifting an empty list.
func TestMembersFromListInstancesResponseEmpty(t *testing.T) {
	if got := membersFromListInstancesResponse(map[string]interface{}{}); len(got) != 0 {
		t.Errorf("expected no members, got %v", got)
	}
}

func TestDiffMembers(t *testing.T) {
	tests := []struct {
		name             string
		desired, current []string
		wantAdd, wantRem []string
	}{
		{"add one", []string{vmA, vmB}, []string{vmA}, []string{vmB}, nil},
		{"remove one", []string{vmA}, []string{vmA, vmB}, nil, []string{vmB}},
		{"mixed add+remove", []string{vmA, vmC}, []string{vmA, vmB}, []string{vmC}, []string{vmB}},
		{"no change", []string{vmA, vmB}, []string{vmB, vmA}, nil, nil},
		{"attach to empty", []string{vmA}, nil, []string{vmA}, nil},
		{"detach all", nil, []string{vmA, vmB}, nil, []string{vmA, vmB}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			add, rem := diffMembers(tt.desired, tt.current)
			if !reflect.DeepEqual(add, tt.wantAdd) {
				t.Errorf("toAdd: got %v, want %v", add, tt.wantAdd)
			}
			if !reflect.DeepEqual(rem, tt.wantRem) {
				t.Errorf("toRemove: got %v, want %v", rem, tt.wantRem)
			}
		})
	}
}

// addInstances / removeInstances share the body shape {"instances":[{"instance":url}]}.
func TestInstancesVerbBody(t *testing.T) {
	got := instancesVerbBody([]string{vmA, vmB})
	want := map[string]interface{}{
		"instances": []map[string]interface{}{
			{"instance": vmA},
			{"instance": vmB},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// setNamedPorts carries the fingerprint read back from the group (optimistic lock).
func TestSetNamedPortsBody(t *testing.T) {
	ports := []interface{}{
		map[string]interface{}{"name": "formae", "port": float64(49684)},
	}
	got := setNamedPortsBody(ports, "fp-123")
	want := map[string]interface{}{
		"namedPorts":  ports,
		"fingerprint": "fp-123",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestDesiredMembers(t *testing.T) {
	props := map[string]interface{}{
		"name":      "ig",
		"instances": []interface{}{vmB, vmA},
	}
	got := desiredMembers(props)
	want := []string{vmA, vmB} // sorted
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if m := desiredMembers(map[string]interface{}{"name": "ig"}); len(m) != 0 {
		t.Errorf("absent instances should yield no members, got %v", m)
	}
}

// The read merges live membership into props: sorted members when present,
// field omitted when the group is empty (so it equals an absent desired field).
func TestMergeMembershipIntoProps(t *testing.T) {
	props := map[string]interface{}{"name": "ig"}
	mergeMembershipIntoProps(props, []string{vmB, vmA})
	got, _ := props["instances"].([]string)
	if want := []string{vmA, vmB}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	empty := map[string]interface{}{"name": "ig"}
	mergeMembershipIntoProps(empty, nil)
	if _, ok := empty["instances"]; ok {
		t.Errorf("empty membership must omit instances, got %#v", empty)
	}
}

// instanceGroups.insert rejects a members payload; Create strips it and attaches
// afterwards (cf. sslCertificate stripping managedDomains before insert).
func TestStripInstancesForInsert(t *testing.T) {
	body, err := stripInstancesForInsert(map[string]interface{}{
		"name":      "ig",
		"zone":      "z",
		"instances": []interface{}{vmA},
	}, base.TransformContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := body["instances"]; ok {
		t.Errorf("instances must be stripped from insert body: %#v", body)
	}
	if body["name"] != "ig" || body["zone"] != "z" {
		t.Errorf("non-member fields must survive: %#v", body)
	}
}

// The reconcile plan (desired members + named ports) rides in the RequestID
// alongside the pending operation path, so Status can re-derive the next verb
// statelessly across polls and restarts.
func TestEncodeDecodeReconcile(t *testing.T) {
	np := []interface{}{map[string]interface{}{"name": "formae", "port": float64(49684)}}
	rid := encodeReconcile("projects/p/zones/z/operations/op-1", []string{vmB, vmA}, np)

	opPath, members, ports, ok := decodeReconcile(rid)
	if !ok {
		t.Fatal("decode failed")
	}
	if opPath != "projects/p/zones/z/operations/op-1" {
		t.Errorf("opPath: got %q", opPath)
	}
	if !reflect.DeepEqual(members, []string{vmB, vmA}) {
		t.Errorf("members: got %v", members)
	}
	if !namedPortsEqual(ports, np) {
		t.Errorf("namedPorts: got %v", ports)
	}
}

func TestDecodeReconcileEmptyOpAndNoMembers(t *testing.T) {
	// Update encodes an empty op path (Status reconciles immediately).
	opPath, members, ports, ok := decodeReconcile(encodeReconcile("", nil, nil))
	if !ok || opPath != "" || len(members) != 0 || len(ports) != 0 {
		t.Errorf("got op=%q members=%v ports=%v ok=%v", opPath, members, ports, ok)
	}
}

func TestDecodeReconcileNotOurs(t *testing.T) {
	// A bare operation path (no reconcile suffix) is not ours -> ok=false.
	if _, _, _, ok := decodeReconcile("projects/p/zones/z/operations/op-1"); ok {
		t.Error("bare op path should decode ok=false")
	}
}

func TestNamedPortsEqual(t *testing.T) {
	a := []interface{}{map[string]interface{}{"name": "formae", "port": float64(49684)}}
	b := []interface{}{map[string]interface{}{"name": "formae", "port": float64(49684)}}
	c := []interface{}{map[string]interface{}{"name": "formae", "port": float64(8080)}}
	if !namedPortsEqual(a, b) {
		t.Errorf("identical named ports should be equal")
	}
	if namedPortsEqual(a, c) {
		t.Errorf("differing ports should not be equal")
	}
	if !namedPortsEqual(nil, nil) {
		t.Errorf("both absent should be equal")
	}
	if namedPortsEqual(a, nil) {
		t.Errorf("present vs absent should differ")
	}
}
