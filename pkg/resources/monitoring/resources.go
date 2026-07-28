// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package monitoring implements GCP Cloud Monitoring resources.
package monitoring

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

const NotificationChannelResourceType = "GCP::Monitoring::NotificationChannel"

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
	})
	if err != nil {
		panic(err)
	}
}
