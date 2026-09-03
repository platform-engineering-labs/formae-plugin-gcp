// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package kms implements GCP Cloud KMS resources.
package kms

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const KeyRingResourceType = "GCP::KMS::KeyRing"

var kmsRegistry *base.ResourceRegistry

func init() {
	kmsRegistry = base.NewResourceRegistry(KMSAPI, KMSOperations, KMSNativeID)

	// Cloud KMS KeyRings cannot be updated — the API exposes no keyRings.patch
	// method, and the only declarable field is the immutable id, so a change
	// replaces.
	//
	// Delete, on the other hand, now exists: keyRings.delete was added to the
	// v1 API and takes effect immediately - it answers with an Operation whose
	// work is already finished, and a GET on the ring 404s on the very next
	// call, which is why the synchronous OperationConfig below is right for it.
	// A deleted ring's id can also be re-used, so an out-of-band delete followed
	// by a re-apply of the same forma succeeds. Registering it stops every
	// created ring being permanent: without it a key ring could only ever be
	// abandoned, which is exactly why this type's conformance case had to be
	// dropped.
	err := kmsRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: KeyRingResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "keyRings",
				Scope:         &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam: "keyRingId", // id goes in ?keyRingId=, not the body
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
