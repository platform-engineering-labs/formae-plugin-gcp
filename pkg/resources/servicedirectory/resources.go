// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package servicedirectory

import (
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

const (
	NamespaceResourceType = "GCP::ServiceDirectory::Namespace"
	ServiceResourceType   = "GCP::ServiceDirectory::Service"
	EndpointResourceType  = "GCP::ServiceDirectory::Endpoint"
)

var serviceDirectoryRegistry *base.ResourceRegistry

func init() {
	serviceDirectoryRegistry = base.NewResourceRegistry(
		ServiceDirectoryAPI, ServiceDirectoryOperations, ServiceDirectoryNativeID)

	err := serviceDirectoryRegistry.RegisterAll([]base.ResourceDefinition{
		{
			// A namespace is the top-level container and the only one of the
			// three that lists at the location, so it needs no override.
			ResourceType: NamespaceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "namespaces",
				Scope:              &base.ScopeConfig{Type: base.ScopeLocationBased},
				CreateIDParam:      "namespaceId", // id goes in ?namespaceId=, not the body
				SupportsUpdate:     true,
				UpdateMaskFromBody: true, // PATCH ?updateMask=<body fields>
			},
			RequestTransformer:  base.DropFieldsOnUpdate("name"),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			ResourceType: ServiceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "services",
				Scope:        &base.ScopeConfig{Type: base.ScopeLocationBased},
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "namespaces",
					PropertyName:   "namespace",
					RequiresParent: true,
				},
				CreateIDParam:      "serviceId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer: &base.CompositeRequestTransformer{Transformers: []base.RequestTransformer{
				// "namespace" addresses the service rather than describing it;
				// the API rejects it as an unknown body field. "name" is dropped
				// on update only - create reads the id (?serviceId=) from it.
				base.DropFields("namespace"),
				base.DropFieldsOnUpdate("name"),
			}},
			ResponseTransformer: base.ResponseTransformerFunc(serviceResponseTransformer),
		},
		{
			ResourceType: EndpointResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "endpoints",
				Scope:        &base.ScopeConfig{Type: base.ScopeLocationBased},
				ParentResource: &base.ParentResourceConfig{
					ParentType:              "services",
					PropertyName:            "service",
					RequiresParent:          true,
					GrandParentType:         "namespaces",
					GrandParentPropertyName: "namespace",
				},
				CreateIDParam:      "endpointId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer: &base.CompositeRequestTransformer{Transformers: []base.RequestTransformer{
				base.DropFields("namespace", "service"),
				base.DropFieldsOnUpdate("name"),
			}},
			ResponseTransformer: base.ResponseTransformerFunc(endpointResponseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}

	registerParentWalkingLists()
}

// serviceResponseTransformer puts back what the API leaves in the resource path.
// A service reports only its full name, so the namespace a forma declares would
// otherwise look absent and every read would plan a change.
func serviceResponseTransformer(props map[string]interface{}, _ base.TransformContext) map[string]interface{} {
	out := copyProps(props)
	// projects/{p}/locations/{l}/namespaces/{ns}/services/{svc}
	if parts := pathParts(props, 8); parts != nil && parts[6] == "services" {
		out["namespace"] = parts[5]
		out["name"] = parts[7]
	}
	return out
}

// endpointResponseTransformer does the same one level deeper: an endpoint's
// namespace and service both live only in its path.
func endpointResponseTransformer(props map[string]interface{}, _ base.TransformContext) map[string]interface{} {
	out := copyProps(props)
	// projects/{p}/locations/{l}/namespaces/{ns}/services/{svc}/endpoints/{ep}
	if parts := pathParts(props, 10); parts != nil && parts[6] == "services" && parts[8] == "endpoints" {
		out["namespace"] = parts[5]
		out["service"] = parts[7]
		out["name"] = parts[9]
	}
	return out
}

func copyProps(props map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(props)+2)
	for k, v := range props {
		out[k] = v
	}
	return out
}

// pathParts splits a response's full-path "name" when it has exactly n
// segments, and returns nil otherwise - a short name has already been
// transformed, or the response is not the shape this transformer handles.
func pathParts(props map[string]interface{}, n int) []string {
	name, _ := props["name"].(string)
	if name == "" {
		return nil
	}
	parts := strings.Split(name, "/")
	if len(parts) != n || parts[2] != "locations" || parts[4] != "namespaces" {
		return nil
	}
	return parts
}
