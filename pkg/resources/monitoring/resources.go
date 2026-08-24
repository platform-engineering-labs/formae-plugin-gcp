// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package monitoring implements GCP Cloud Monitoring resources.
package monitoring

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const (
	NotificationChannelResourceType = "GCP::Monitoring::NotificationChannel"
	GroupResourceType               = "GCP::Monitoring::Group"
	UptimeCheckConfigResourceType   = "GCP::Monitoring::UptimeCheckConfig"
	AlertPolicyResourceType         = "GCP::Monitoring::AlertPolicy"
	DashboardResourceType           = "GCP::Monitoring::Dashboard"
	CustomServiceResourceType       = "GCP::Monitoring::CustomService"
	SloResourceType                 = "GCP::Monitoring::Slo"
	MetricDescriptorResourceType    = "GCP::Monitoring::MetricDescriptor"
)

// The API field "type" (e.g. "email", "slack") collides with formae.Resource's
// reserved "type" (the GCP::Svc::Resource string). The PKL schema exposes it as
// "channelType"; these transformers rename it on the wire.
var channelTypeToAPI = base.RequestTransformerFunc(
	func(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
		v, ok := props["channelType"]
		if !ok {
			return props, nil
		}
		out := make(map[string]interface{}, len(props))
		for k, val := range props {
			if k == "channelType" {
				continue
			}
			out[k] = val
		}
		out["type"] = v
		return out, nil
	})

var channelTypeFromAPI = base.ResponseTransformerFunc(
	func(resp map[string]interface{}, _ base.TransformContext) map[string]interface{} {
		if v, ok := resp["type"]; ok {
			resp["channelType"] = v
			delete(resp, "type")
		}
		return resp
	})

// dashboards.create requires the fully-qualified name
// ("projects/{project}/dashboards/{id}") in the body; the schema declares the
// short id so the forma reads like every other resource. Expanding it here is
// what lets the caller choose the dashboard ID instead of taking a
// server-generated one, which in turn keeps `name` stable for reconciliation.
var dashboardNameToAPI = base.RequestTransformerFunc(
	func(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
		name, ok := props["name"].(string)
		if !ok || name == "" || strings.HasPrefix(name, "projects/") {
			return props, nil
		}
		out := make(map[string]interface{}, len(props))
		for k, v := range props {
			out[k] = v
		}
		out["name"] = fmt.Sprintf("projects/%s/dashboards/%s", ctx.Project, name)
		return out, nil
	})

// A Monitoring Service must declare which flavour it is. For a hand-declared
// ("custom") service that means an empty "custom" object in the create body —
// formae prunes empty sub-objects before they reach the plugin, so inject it
// here rather than making callers write `custom = new Mapping {}`.
var customServiceMarker = base.RequestTransformerFunc(
	func(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
		if ctx.Operation != resource.OperationCreate {
			return props, nil
		}
		if _, ok := props["custom"]; ok {
			return props, nil
		}
		out := make(map[string]interface{}, len(props)+1)
		for k, v := range props {
			out[k] = v
		}
		out["custom"] = map[string]interface{}{}
		return out, nil
	})

// An SLO response carries no "service" field — the owning service is only
// visible inside the full resource name. Lift it back out so the stored state
// matches the declared forma, then shorten the name to the SLO id.
var sloServiceFromName = base.ResponseTransformerFunc(
	func(resp map[string]interface{}, _ base.TransformContext) map[string]interface{} {
		name, ok := resp["name"].(string)
		if !ok {
			return resp
		}
		parts := strings.Split(name, "/")
		for i := 0; i+1 < len(parts); i++ {
			if parts[i] == "services" {
				resp["service"] = parts[i+1]
				break
			}
		}
		return resp
	})

// A metric descriptor is identified by its metric type
// ("custom.googleapis.com/foo"). The API field is "type", which formae's base
// Resource class reserves, so the schema calls it "name" — that also lines up
// with the identifier every other resource uses, and the slashes ride through
// the URL path unchanged.
var metricDescriptorNameToType = base.RequestTransformerFunc(
	func(props map[string]interface{}, _ base.TransformContext) (map[string]interface{}, error) {
		name, ok := props["name"]
		if !ok {
			return props, nil
		}
		out := make(map[string]interface{}, len(props))
		for k, v := range props {
			if k == "name" {
				continue
			}
			out[k] = v
		}
		out["type"] = name
		return out, nil
	})

// On read the descriptor comes back with both a full-path "name" and the
// "type"; keep the type under the schema's "name" and drop the API field.
var metricDescriptorTypeToName = base.ResponseTransformerFunc(
	func(resp map[string]interface{}, _ base.TransformContext) map[string]interface{} {
		if t, ok := resp["type"].(string); ok && t != "" {
			resp["name"] = t
			delete(resp, "type")
		}
		return resp
	})

var monitoringRegistry *base.ResourceRegistry

func init() {
	monitoringRegistry = base.NewResourceRegistry(MonitoringAPI, MonitoringOperations, MonitoringNativeID)

	err := monitoringRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: NotificationChannelResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "notificationChannels",
				Scope:              &base.ScopeConfig{Type: base.ScopeProjectLevel},
				SupportsUpdate:     true,
				UpdateMethod:       base.UpdateMethodPatch,
				UpdateMaskFromBody: true, // notificationChannels.patch requires updateMask
			},
			RequestTransformer: channelTypeToAPI,
			// Shorten the server-assigned full-path name, then map type->channelType.
			ResponseTransformer: &base.CompositeResponseTransformer{
				Transformers: []base.ResponseTransformer{
					base.ShortNameResponseTransformer,
					channelTypeFromAPI,
				},
			},
		},
		{
			ResourceType: GroupResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "groups",
				Scope:          &base.ScopeConfig{Type: base.ScopeProjectLevel},
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPut, // groups.update is a full PUT
				// groups.list returns {"group":[...]}, not "items"/"groups";
				// without this discovery lists 0 and the resource never appears.
				ListItemsKey: "group",
			},
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			ResourceType: UptimeCheckConfigResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "uptimeCheckConfigs",
				Scope:              &base.ScopeConfig{Type: base.ScopeProjectLevel},
				SupportsUpdate:     true,
				UpdateMethod:       base.UpdateMethodPatch,
				UpdateMaskFromBody: true,
			},
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			ResourceType: AlertPolicyResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:       "alertPolicies",
				Scope:              &base.ScopeConfig{Type: base.ScopeProjectLevel},
				SupportsUpdate:     true,
				UpdateMethod:       base.UpdateMethodPatch,
				UpdateMaskFromBody: true,
			},
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		{
			ResourceType: DashboardResourceType,
			// Dashboards are the one Monitoring resource on v1.
			APIConfig: MonitoringDashboardAPI,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "dashboards",
				Scope:        &base.ScopeConfig{Type: base.ScopeProjectLevel},
				// ponytail: update is off for now. dashboards.patch replaces the
				// whole dashboard, takes no updateMask (unlike its v3 siblings),
				// and requires the current etag in the body — verified against
				// the live API: without one it returns 400 "Update Dashboard
				// should specify a non empty etag.", and with one taken from a
				// fresh GET it succeeds. Configuring the generic engine's
				// optimistic-locking path (FieldName "etag", body location)
				// still failed conformance's Update step, and the plugin's
				// failure StatusMessage is not surfaced anywhere readable, so
				// the cause is not yet identified. A change replaces instead.
				SupportsUpdate: false,
			},
			RequestTransformer:  dashboardNameToAPI,
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		// Custom Service - the SLO container. The id goes in a create-time
		// query param, not the body.
		{
			ResourceType: CustomServiceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:  "services",
				Scope:         &base.ScopeConfig{Type: base.ScopeProjectLevel},
				CreateIDParam: "serviceId",
				// ponytail: services.patch works but its updateMask would name
				// "custom", which is not independently mutable. displayName is
				// the only field worth changing; wire it when someone asks.
				SupportsUpdate: false,
			},
			RequestTransformer:  customServiceMarker,
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
		// SLO - nested under a service:
		// /projects/{p}/services/{service}/serviceLevelObjectives/{id}
		{
			ResourceType:   SloResourceType,
			NativeIDConfig: MonitoringSloNativeID,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "serviceLevelObjectives",
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "services",
					PropertyName:   "service",
					RequiresParent: true,
				},
				CreateIDParam:      "serviceLevelObjectiveId",
				SupportsUpdate:     true,
				UpdateMethod:       base.UpdateMethodPatch,
				UpdateMaskFromBody: true,
			},
			// "service" is a path component, not a body field: leaving it in an
			// update body would put it in the updateMask and the API would
			// reject the call.
			// "service" identifies the owning service in the URL path, not in
			// the payload, and Monitoring rejects unknown body fields. "name"
			// only needs dropping on update — on create CreateIDParam has
			// already moved it to the serviceLevelObjectiveId query param.
			RequestTransformer: &base.CompositeRequestTransformer{
				Transformers: []base.RequestTransformer{
					base.DropFields("service"),
					base.DropFieldsOnUpdate("name"),
				},
			},
			ResponseTransformer: &base.CompositeResponseTransformer{
				Transformers: []base.ResponseTransformer{
					sloServiceFromName,
					base.ShortNameResponseTransformer,
				},
			},
		},
		// MetricDescriptor - a custom metric's schema. No patch endpoint exists,
		// so a change replaces.
		{
			ResourceType:   MetricDescriptorResourceType,
			NativeIDConfig: MonitoringMetricDescriptorNativeID,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "metricDescriptors",
				Scope:          &base.ScopeConfig{Type: base.ScopeProjectLevel},
				SupportsUpdate: false,
			},
			RequestTransformer:  metricDescriptorNameToType,
			ResponseTransformer: metricDescriptorTypeToName,
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
