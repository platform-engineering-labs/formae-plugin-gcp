// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package logging

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestPathBuilder(t *testing.T) {
	ctx := base.PathContext{Project: "p", ResourceType: "metrics"}
	if got := loggingPathBuilder(ctx); got != "/projects/p/metrics" {
		t.Errorf("collection path = %q", got)
	}
	ctx.ResourceName = "error_count"
	if got := loggingPathBuilder(ctx); got != "/projects/p/metrics/error_count" {
		t.Errorf("resource path = %q", got)
	}
}

func TestNativeIDFromContext(t *testing.T) {
	ctx := base.PathContext{Project: "p", ResourceType: "metrics", ResourceName: "error_count"}
	if got := extractLoggingNativeID(map[string]interface{}{}, ctx); got != "projects/p/metrics/error_count" {
		t.Errorf("native id from ctx = %q", got)
	}
}

func TestNativeIDFromResponse(t *testing.T) {
	// When ctx has no name (e.g. discovery), fall back to the response "name".
	ctx := base.PathContext{Project: "p", ResourceType: "metrics"}
	got := extractLoggingNativeID(map[string]interface{}{"name": "error_count"}, ctx)
	if got != "projects/p/metrics/error_count" {
		t.Errorf("native id from response = %q", got)
	}
}

func TestParseNativeID(t *testing.T) {
	ctx, err := parseLoggingNativeID("projects/p/metrics/error_count")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.Project != "p" || ctx.ResourceType != "metrics" || ctx.ResourceName != "error_count" {
		t.Errorf("parsed ctx = %+v", ctx)
	}
	if _, err := parseLoggingNativeID("invalid/id"); err == nil {
		t.Errorf("expected error for malformed native ID")
	}
}

func TestLogMetricRegistered(t *testing.T) {
	for _, op := range []resource.Operation{
		resource.OperationCreate, resource.OperationRead,
		resource.OperationDelete, resource.OperationList,
	} {
		if !registry.HasProvisioner(LogMetricResourceType, op) {
			t.Errorf("%s not registered for %v", LogMetricResourceType, op)
		}
	}
	// Update is deferred; it must NOT be registered.
	if registry.HasProvisioner(LogMetricResourceType, resource.OperationUpdate) {
		t.Errorf("%s should not register Update (SupportsUpdate:false)", LogMetricResourceType)
	}
}
