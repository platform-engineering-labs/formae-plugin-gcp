// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package redis

import (
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// The registry only keeps a definition's own OperationConfig when its
// OperationIDExtractor is non-nil (see ResourceRegistry.Register). Miss that and
// the ACL policy would silently run on the LRO path, where a create reports
// InProgress with no operation to poll.
func TestAclPolicyUsesSynchronousOperations(t *testing.T) {
	def, ok := redisRegistry.Definitions[AclPolicyResourceType]
	if !ok {
		t.Fatalf("%s not in registry", AclPolicyResourceType)
	}
	if !def.OperationConfig.Synchronous {
		t.Error("acl policy must use the synchronous override, not the registry's LRO config")
	}
	if inst := redisRegistry.Definitions[InstanceResourceType]; inst.OperationConfig.Synchronous {
		t.Error("instance must stay asynchronous")
	}
}

func TestAclPolicyResponseTransformer(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]interface{}
		want map[string]interface{}
	}{
		{
			name: "full path name is shortened to the declared id",
			in: map[string]interface{}{
				"name":  "projects/p/locations/europe-central2/aclPolicies/my-acl",
				"state": "ACTIVE",
			},
			want: map[string]interface{}{"name": "my-acl", "state": "ACTIVE"},
		},
		{
			name: "nested output-only attachments are stripped",
			in: map[string]interface{}{
				"name": "projects/p/locations/europe-central2/aclPolicies/my-acl",
				"clusterAclPolicyAttachments": []interface{}{
					map[string]interface{}{
						"cluster": "projects/p/locations/europe-central2/clusters/c1",
					},
				},
				"rules": []interface{}{
					map[string]interface{}{"username": "u@p.iam.gserviceaccount.com", "rule": "on ~* +@all"},
				},
			},
			want: map[string]interface{}{
				"name": "my-acl",
				"rules": []interface{}{
					map[string]interface{}{"username": "u@p.iam.gserviceaccount.com", "rule": "on ~* +@all"},
				},
			},
		},
		{
			name: "an already-short name is left alone",
			in:   map[string]interface{}{"name": "my-acl"},
			want: map[string]interface{}{"name": "my-acl"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := aclPolicyResponseTransformer(tc.in, base.TransformContext{Project: "p"})
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAclPolicyRequestTransformer(t *testing.T) {
	rules := []interface{}{
		map[string]interface{}{"username": "u@p.iam.gserviceaccount.com", "rule": "on ~* +@all"},
	}
	props := func() map[string]interface{} {
		return map[string]interface{}{
			"name":       "my-acl",
			"rules":      rules,
			"state":      "ACTIVE",
			"version":    "3",
			"etag":       "abc",
			"createTime": "2026-01-01T00:00:00Z",
			"updateTime": "2026-01-02T00:00:00Z",
			"clusterAclPolicyAttachments": []interface{}{
				map[string]interface{}{"cluster": "c1"},
			},
		}
	}

	tests := []struct {
		name string
		op   resource.Operation
		want map[string]interface{}
	}{
		{
			// base reads the id out of body["name"] for ?aclPolicyId= and
			// removes it itself, so create must still carry it.
			name: "create keeps name, drops every server-owned field",
			op:   resource.OperationCreate,
			want: map[string]interface{}{"name": "my-acl", "rules": rules},
		},
		{
			// UpdateMaskFromBody builds the mask from what is left, so what is
			// left has to be exactly the mutable field.
			name: "update drops name too, leaving rules as the whole mask",
			op:   resource.OperationUpdate,
			want: map[string]interface{}{"rules": rules},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := aclPolicyRequestTransformer(props(), base.TransformContext{
				Project: "p", Operation: tc.op,
			})
			if err != nil {
				t.Fatalf("transform: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// The declared id must survive the create request body, the API's full-path
// echo, and the response transformer unchanged - otherwise state and forma
// disagree forever and every re-apply plans a replacement.
func TestAclPolicyNameRoundTrip(t *testing.T) {
	const id = "formae-test-redis-acl-abc123"

	body, err := aclPolicyRequestTransformer(
		map[string]interface{}{"name": id, "rules": []interface{}{}},
		base.TransformContext{Project: "p", Operation: resource.OperationCreate})
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	if body["name"] != id {
		t.Fatalf("create body name = %v, want %q", body["name"], id)
	}

	apiEcho := map[string]interface{}{
		"name": "projects/p/locations/europe-central2/aclPolicies/" + id,
	}
	got := aclPolicyResponseTransformer(apiEcho, base.TransformContext{Project: "p"})
	if got["name"] != id {
		t.Errorf("round trip name = %v, want %q", got["name"], id)
	}
}

func TestAclPolicyNativeIDFromSyncCreate(t *testing.T) {
	// A synchronous create returns the resource itself, so the native ID comes
	// straight from its full-path name.
	ctx := base.PathContext{
		Project: "p", Location: "europe-central2", ResourceType: "aclPolicies", ResourceName: "my-acl",
	}
	got := RedisSyncOperations.NativeIDExtractor(map[string]interface{}{
		"name":  "projects/p/locations/europe-central2/aclPolicies/my-acl",
		"state": "ACTIVE",
	}, ctx)
	if want := "projects/p/locations/europe-central2/aclPolicies/my-acl"; got != want {
		t.Errorf("native id = %q, want %q", got, want)
	}
}

func TestAclPolicyRegistered(t *testing.T) {
	for _, op := range []resource.Operation{
		resource.OperationCreate, resource.OperationRead, resource.OperationUpdate,
		resource.OperationDelete, resource.OperationList,
	} {
		if !registry.HasProvisioner(AclPolicyResourceType, op) {
			t.Errorf("%s not registered for %v", AclPolicyResourceType, op)
		}
	}
}

// A read that comes back with the API's tombstone must be reported as missing,
// or a synchronization inside the fifteen-to-twenty-second window after an
// accepted delete restores a deleted policy to inventory.
func TestAclPolicyDeletingIsTreatedAsMissing(t *testing.T) {
	tests := []struct {
		name string
		body map[string]interface{}
		want bool
	}{
		{"tombstone", map[string]interface{}{"state": "DELETING"}, true},
		{"live", map[string]interface{}{"state": "ACTIVE"}, false},
		{"no state at all", map[string]interface{}{"rules": []interface{}{}}, false},
		{"state is not a string", map[string]interface{}{"state": 3}, false},
		{"empty body", map[string]interface{}{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := aclPolicyDeleting(tc.body); got != tc.want {
				t.Errorf("aclPolicyDeleting(%v) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}
