// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package monitoring

import (
	"errors"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// metricDescriptors.create answers 200 with the descriptor, but a GET on it
// 404s for a couple of seconds afterwards. Measured against the live API on
// 2026-09-08, project development-477117:
//
//	create -> 200
//	t=0s   -> 404
//	t=1s   -> 404
//	t=2s   -> 200
//
// The API config declares Synchronous: true, so base reports the create
// complete straight from the create response and Status is a no-op. A sync
// landing inside that window read "not found" and the agent tombstoned a
// descriptor that existed - the nightly's [Sync] failure on 2026-09-08, which
// passed on 09-07 and twice on 09-06 because the window is short.
func TestMetricDescriptorCreateIsWrappedForReadback(t *testing.T) {
	provisioner := registry.Get(MetricDescriptorResourceType, resource.OperationCreate, &config.Config{})
	if provisioner == nil {
		t.Fatal("no Create provisioner registered")
	}
	if _, ok := provisioner.(*metricDescriptorProvisioner); !ok {
		t.Errorf("Create provisioner is %T, want *metricDescriptorProvisioner - a bare "+
			"BaseResource reports success before the descriptor is readable", provisioner)
	}
}

// Only Create needs the readback. Read must keep reporting a genuine absence
// immediately, or out-of-band delete detection slows down for every resource;
// and Status must stay untouched, since StatusRequest carries no operation and
// so cannot tell a post-create check from a post-delete one - confirming
// readability there would poll forever after a Destroy.
func TestOtherMetricDescriptorOperationsAreNotWrapped(t *testing.T) {
	for _, op := range []resource.Operation{
		resource.OperationRead, resource.OperationDelete,
		resource.OperationList, resource.OperationCheckStatus,
	} {
		provisioner := registry.Get(MetricDescriptorResourceType, op, &config.Config{})
		if provisioner == nil {
			t.Fatalf("no provisioner for %v", op)
		}
		if _, ok := provisioner.(*metricDescriptorProvisioner); ok {
			t.Errorf("%v is wrapped for readback; only Create should be", op)
		}
	}
}

// The readback gives up rather than failing a create that actually succeeded.
// The descriptor exists either way - the POST returned it - so a still-absent
// read after the budget means the API is slower than measured, not that the
// create failed.
func TestReadbackTreatsOnlyNotFoundAsUnsettled(t *testing.T) {
	cases := map[string]struct {
		result   *resource.ReadResult
		err      error
		unsettle bool
	}{
		"not found yet":   {&resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil, true},
		"readable":        {&resource.ReadResult{Properties: `{"name":"custom.googleapis.com/x"}`}, nil, false},
		"transport error": {nil, errors.New("connection reset"), true},
		"other API error": {&resource.ReadResult{ErrorCode: resource.OperationErrorCodeInvalidRequest}, nil, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := readbackUnsettled(tc.result, tc.err); got != tc.unsettle {
				t.Errorf("readbackUnsettled = %v, want %v", got, tc.unsettle)
			}
		})
	}
}
