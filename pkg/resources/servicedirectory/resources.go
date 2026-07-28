// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package servicedirectory implements GCP Service Directory resources.
package servicedirectory

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const NamespaceResourceType = "GCP::ServiceDirectory::Namespace"

var serviceDirectoryRegistry *base.ResourceRegistry

func init() {
	serviceDirectoryRegistry = base.NewResourceRegistry(
		ServiceDirectoryAPI, ServiceDirectoryOperations, ServiceDirectoryNativeID)

	// ponytail: Update deferred (as Artifact Registry does). namespaces.patch
	// only mutates labels while name is immutable, so an updateMask must not
	// name it; defer Update until that shaping is proven live. create/read/
	// delete/list are the synchronous, location-scoped paths this batch adds.
	err := serviceDirectoryRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: NamespaceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "namespaces",
				Scope:         &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam: "namespaceId", // id goes in ?namespaceId=, not the body
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
	})
	if err != nil {
		panic(err)
	}
}
