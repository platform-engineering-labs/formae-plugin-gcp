// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
)

func init() {
	registry.Register("GCP::Compute::Instance",
		[]resource.Operation{
			resource.OperationCreate,
			resource.OperationRead,
			resource.OperationUpdate,
			resource.OperationDelete,
			resource.OperationList,
			resource.OperationCheckStatus,
		},
		func(cfg *config.Config) prov.Provisioner {
			prov, err := NewComputeProvisioner(cfg, InstanceResourceType)
			if err != nil {
				panic(err)
			}
			return prov
		})
}
