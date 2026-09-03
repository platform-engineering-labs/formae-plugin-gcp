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
				// contacts.patch exists and works - an updateMask of
				// "languageTag,notificationCategorySubscriptions" is accepted,
				// and one naming "email" is refused, so email is immutable and
				// the other two are the patchable pair. It stays deferred only
				// because nothing exercises it.
				//
				// What stops a case: the shared test project's Allowed Contact
				// Domains org policy admits exactly one domain - the
				// organisation's own live mail domain, with real Google
				// Workspace MX records. No subdomain of it is admitted, and
				// example.com is not. So every address a case could use is an
				// invented mailbox on a domain real people read, and choosing to
				// point Google's technical notifications there is not a decision
				// this plugin should make on its own. An Allowed Contact Domains
				// entry for a throwaway domain would unblock the case, and with
				// it this field.
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
