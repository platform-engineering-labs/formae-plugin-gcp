// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package cloudtasks implements GCP Cloud Tasks resources.
package cloudtasks

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

const QueueResourceType = "GCP::CloudTasks::Queue"

// TasksAPI - Cloud Tasks API v2. Queues are location-scoped and
// create/get/delete are synchronous (the mutated Queue is returned directly).
var TasksAPI = base.APIConfig{
	BaseURL:     "https://cloudtasks.googleapis.com/v2",
	APIVersion:  "v2",
	PathBuilder: base.LocationPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

var TasksOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      base.LocationNativeIDExtractor,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// TasksNativeID - full path "projects/{project}/locations/{location}/queues/{name}".
var TasksNativeID = base.NativeIDConfig{
	Format:       base.FullPathFormat,
	PathTemplate: "projects/{project}/locations/{location}/{resourceType}/{name}",
}

var tasksRegistry *base.ResourceRegistry

func init() {
	tasksRegistry = base.NewResourceRegistry(TasksAPI, TasksOperations, TasksNativeID)

	// Cloud Tasks wants the fully-qualified name in the create body; users
	// declare the short name, so expand on the way in and shorten on the way out.
	// ponytail: SupportsUpdate deferred until PATCH+updateMask is verified live.
	err := tasksRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: QueueResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "queues",
				Scope:          &base.ScopeConfig{Type: base.ScopeLocationBased},
				SupportsUpdate: false,
			},
			RequestTransformer:  base.FullResourceNameExpander(),
			ResponseTransformer: base.ShortNameResponseTransformer,
		},
	})
	if err != nil {
		panic(err)
	}
}
