// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package monitoring

import (
	"context"
	"time"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// A freshly created metric descriptor is not immediately readable. Measured
// against the live API on 2026-09-08 (project development-477117):
//
//	POST  metricDescriptors        -> 200, returns the descriptor
//	GET   metricDescriptors/{type} -> 404 at t=0s, 404 at t=1s, 200 from t=2s
//
// MonitoringOperations declares Synchronous: true, so base reports the create
// complete from the create response and Status is a no-op - nothing else in
// the plugin waits. A synchronization landing inside that window read "not
// found", and formae tombstoned a descriptor that existed: the nightly's
// [Sync] failure on 2026-09-08, which passed on 09-07 and twice on 09-06
// because the window is only a second or two wide.
//
// That is worse than a red nightly. On a live installation the same race
// removes a managed resource from the inventory on nothing but a timing
// coincidence, and the next reconcile recreates or orphans it.
//
// So confirm the descriptor is readable before reporting the create complete.
// This is deliberately on Create rather than anywhere more general:
//
//   - Read must keep reporting a genuine absence immediately, or out-of-band
//     delete detection slows down for every resource in the plugin.
//   - Status cannot do it: StatusRequest carries no operation, so it cannot
//     tell a post-create check from a post-delete one, and confirming
//     readability there would poll forever after a Destroy.
const (
	readbackBudget   = 20 * time.Second
	readbackInterval = 1 * time.Second
)

type metricDescriptorProvisioner struct {
	*base.BaseResource
}

// registerMetricDescriptorReadback re-registers only Create behind the
// readback. Called from the package init in resources.go rather than from an
// init here: Go runs init functions in filename order, and this file sorts
// before "resources.go", so a registration here would be silently replaced by
// the generic one.
func registerMetricDescriptorReadback() {
	registry.Register(MetricDescriptorResourceType,
		[]resource.Operation{resource.OperationCreate},
		func(cfg *config.Config) prov.Provisioner {
			def := monitoringRegistry.Definitions[MetricDescriptorResourceType]
			return &metricDescriptorProvisioner{
				BaseResource: &base.BaseResource{
					Config:              cfg,
					APIConfig:           MonitoringAPI,
					OperationConfig:     MonitoringOperations,
					ResourceConfig:      def.ResourceConfig,
					NativeIDConfig:      MonitoringMetricDescriptorNativeID,
					RequestTransformer:  def.RequestTransformer,
					ResponseTransformer: def.ResponseTransformer,
				},
			}
		})
}

func (m *metricDescriptorProvisioner) Create(
	ctx context.Context,
	request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	result, err := m.BaseResource.Create(ctx, request)
	if err != nil || result == nil || result.ProgressResult == nil {
		return result, err
	}
	if result.ProgressResult.OperationStatus != resource.OperationStatusSuccess {
		return result, nil
	}
	m.waitUntilReadable(ctx, result.ProgressResult.NativeID, request.TargetConfig)
	return result, nil
}

// waitUntilReadable polls the descriptor until a Read finds it, or the budget
// runs out.
//
// It never turns a successful create into a failure. The descriptor exists
// either way - the POST returned it - so a still-absent read after the budget
// means the API is slower than measured, not that the create did not happen.
// Reporting success is then still the truthful answer, and the alternative
// (failing a create that worked) is strictly worse.
func (m *metricDescriptorProvisioner) waitUntilReadable(
	ctx context.Context, nativeID string, targetConfig []byte,
) {
	if nativeID == "" {
		return
	}
	deadline := time.Now().Add(readbackBudget)
	for {
		readResult, readErr := m.Read(ctx, &resource.ReadRequest{
			NativeID:     nativeID,
			ResourceType: MetricDescriptorResourceType,
			TargetConfig: targetConfig,
		})
		if !readbackUnsettled(readResult, readErr) {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(readbackInterval):
		}
	}
}

// readbackUnsettled reports whether a readback attempt says "not visible yet"
// rather than giving a definitive answer. Only a missing resource and a failed
// request are unsettled; any other API error is a real answer and polling past
// it would just delay the create.
func readbackUnsettled(result *resource.ReadResult, err error) bool {
	if err != nil {
		return true
	}
	if result == nil {
		return true
	}
	return result.ErrorCode == resource.OperationErrorCodeNotFound
}
