// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package cloudscheduler implements GCP Cloud Scheduler resources.
package cloudscheduler

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

const JobResourceType = "GCP::CloudScheduler::Job"

// SchedulerAPI - Cloud Scheduler API v1. Jobs are location-scoped and
// create/get/delete are synchronous (the mutated Job is returned directly).
var SchedulerAPI = base.APIConfig{
	BaseURL:     "https://cloudscheduler.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: base.LocationPathBuilder,
	Pagination:  &base.PaginationConfig{PageSizeParam: "pageSize"},
}

var SchedulerOperations = base.OperationConfig{
	Synchronous:            true,
	OperationIDExtractor:   func(map[string]interface{}) string { return "" },
	OperationURLBuilder:    func(base.PathContext, string) string { return "" },
	NativeIDExtractor:      base.LocationNativeIDExtractor,
	OperationStatusChecker: func(map[string]interface{}) (bool, error) { return true, nil },
}

// SchedulerNativeID - full path "projects/{project}/locations/{location}/jobs/{name}".
var SchedulerNativeID = base.NativeIDConfig{
	Format:       base.FullPathFormat,
	PathTemplate: "projects/{project}/locations/{location}/{resourceType}/{name}",
}

var schedulerRegistry *base.ResourceRegistry

func init() {
	schedulerRegistry = base.NewResourceRegistry(SchedulerAPI, SchedulerOperations, SchedulerNativeID)

	// Cloud Scheduler wants the fully-qualified name in the create body; users
	// declare the short name, so expand on the way in and shorten on the way out.
	// ponytail: SupportsUpdate deferred (as CloudRun/DNS do) until PATCH+updateMask
	// is verified against a live project.
	err := schedulerRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: JobResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "jobs",
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
