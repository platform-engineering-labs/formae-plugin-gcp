// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package essentialcontacts

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// ContactResourceType is the Formae resource type for Essential Contacts.
const ContactResourceType = "GCP::EssentialContacts::Contact"

var essentialContactsRegistry *base.ResourceRegistry

func init() {
	essentialContactsRegistry = base.NewResourceRegistry(
		EssentialContactsAPI, EssentialContactsOperations, EssentialContactsNativeID)

	// Contact fits the generic engine: create is a plain POST to the collection
	// (the contact id is server-assigned, so there is no CreateIDParam and no
	// name in the request body); Read/Delete/List operate on the full resource
	// path. The response "name" is a full path shortened to its server-assigned
	// id by ShortNameResponseTransformer. No custom provisioner needed.
	err := essentialContactsRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: ContactResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "contacts",
				Scope:        &base.ScopeConfig{Type: base.ScopeProjectLevel},
				// contacts.patch takes an updateMask naming the fields to
				// change. Enabled now that a conformance case exercises it;
				// email is immutable in practice, so the case changes
				// languageTag.
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			ResponseTransformer: base.ShortNameResponseTransformer,
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationUpdate,
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
