// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package logging

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const LogMetricResourceType = "GCP::Logging::LogMetric"

var loggingRegistry *base.ResourceRegistry

func init() {
	loggingRegistry = base.NewResourceRegistry(LoggingAPI, LoggingOperations, LoggingNativeID)

	// LogMetric fits the generic engine: create is a plain POST to
	// projects/{p}/metrics with the client-assigned metric id in the body's
	// "name" field (no id query param, no transformer). Read/Delete operate on
	// the full resource path; delete returns Empty. All operations are sync.
	// Update (PUT) is deferred (SupportsUpdate: false); conformance skips it.
	err := loggingRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: LogMetricResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "metrics",
				Scope:          &base.ScopeConfig{Type: base.ScopeProjectLevel},
				SupportsUpdate: false,
			},
			ResponseTransformer: base.ShortNameResponseTransformer,
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
		},
	})
	if err != nil {
		panic(err)
	}
}
