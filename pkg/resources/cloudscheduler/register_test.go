// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package cloudscheduler

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// Registering happens in init(); this proves the definition wires up without
// panicking and that the supported CRUD+List operations are queryable.
func TestJobRegistered(t *testing.T) {
	for _, op := range []resource.Operation{
		resource.OperationCreate,
		resource.OperationRead,
		resource.OperationDelete,
		resource.OperationList,
	} {
		if !registry.HasProvisioner(JobResourceType, op) {
			t.Errorf("%s not registered for operation %v", JobResourceType, op)
		}
	}
}
