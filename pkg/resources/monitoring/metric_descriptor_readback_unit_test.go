// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package monitoring

import (
	"context"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// Unmarked Status remains the ordinary synchronous Monitoring contract. The
// readiness wrapper must preserve the caller's NativeID because the base
// synchronous result omits it.
func TestMetricDescriptorUnmarkedStatusPreservesLegacyIdentity(t *testing.T) {
	provisioner := registry.Get(MetricDescriptorResourceType, resource.OperationCheckStatus, &config.Config{})
	if provisioner == nil {
		t.Fatal("no Status provisioner registered")
	}
	result, err := provisioner.Status(context.Background(), &resource.StatusRequest{
		RequestID:    "legacy-operation",
		NativeID:     "projects/p/metricDescriptors/custom.googleapis.com/formae/x",
		ResourceType: MetricDescriptorResourceType,
	})
	if err != nil || result == nil || result.ProgressResult == nil {
		t.Fatalf("Status result=%#v err=%v", result, err)
	}
	progress := result.ProgressResult
	if progress.Operation != resource.OperationCheckStatus || progress.OperationStatus != resource.OperationStatusSuccess ||
		progress.RequestID != "legacy-operation" || progress.NativeID != "projects/p/metricDescriptors/custom.googleapis.com/formae/x" {
		t.Fatalf("legacy Status contract changed: %#v", progress)
	}
}
