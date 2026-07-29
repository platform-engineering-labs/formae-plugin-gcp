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

	// Cloud KMS KeyRings cannot be deleted or updated — the API exposes no
	// keyRings.delete or keyRings.patch method (keys and key material are only
	// ever disabled/destroyed at the CryptoKeyVersion level). Register only the
	// operations the API actually supports: Create/Read/List (+ CheckStatus).
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
