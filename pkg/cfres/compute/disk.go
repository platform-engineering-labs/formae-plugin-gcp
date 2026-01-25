// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
)

func init() {
	// Register Disk with custom provisioner factory to support setLabels update
	registry.Register(DiskResourceType,
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(cfg *config.Config) prov.Provisioner {
			return newDiskProvisioner(cfg)
		})
}

// newDiskProvisioner creates a DiskProvisioner with the custom Update method
func newDiskProvisioner(cfg *config.Config) *DiskProvisioner {
	baseResource := &base.BaseResource{
		Config:    cfg,
		APIConfig: ComputeAPI,
		OperationConfig: ComputeOperations,
		ResourceConfig: base.ResourceConfig{
			ResourceType: "disks",
			Scope: &base.ScopeConfig{
				Type: base.ScopeZonal,
			},
			// Updates are supported via setLabels endpoint
			SupportsUpdate: true,
			OptimisticLocking: &base.OptimisticLockingConfig{
				Enabled:       true,
				FieldName:     "labelFingerprint",
				LocationInURL: false,
			},
		},
		NativeIDConfig:      ComputeNativeID,
		RequestTransformer:  base.RequestTransformerFunc(diskRequestTransformer),
		ResponseTransformer: base.ResponseTransformerFunc(diskResponseTransformer),
	}

	return NewDiskProvisioner(baseResource)
}
