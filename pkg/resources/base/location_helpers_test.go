// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package base

import "testing"

func TestLocationPathBuilder(t *testing.T) {
	ctx := PathContext{Project: "p", Location: "us-central1", ResourceType: "jobs"}
	if got := LocationPathBuilder(ctx); got != "/projects/p/locations/us-central1/jobs" {
		t.Errorf("collection path = %q", got)
	}
	ctx.ResourceName = "j1"
	if got := LocationPathBuilder(ctx); got != "/projects/p/locations/us-central1/jobs/j1" {
		t.Errorf("resource path = %q", got)
	}
}

func TestLocationNativeIDExtractor(t *testing.T) {
	ctx := PathContext{Project: "p", Location: "us-central1", ResourceType: "jobs", ResourceName: "j1"}

	// full name echoed in response
	got := LocationNativeIDExtractor(map[string]interface{}{
		"name": "projects/p/locations/us-central1/jobs/j1",
	}, ctx)
	if got != "projects/p/locations/us-central1/jobs/j1" {
		t.Errorf("from response name = %q", got)
	}

	// name carrying a URL prefix
	got = LocationNativeIDExtractor(map[string]interface{}{
		"name": "https://cloudscheduler.googleapis.com/v1/projects/p/locations/us-central1/jobs/j1",
	}, ctx)
	if got != "projects/p/locations/us-central1/jobs/j1" {
		t.Errorf("from url name = %q", got)
	}

	// response omits name -> built from context
	got = LocationNativeIDExtractor(map[string]interface{}{}, ctx)
	if got != "projects/p/locations/us-central1/jobs/j1" {
		t.Errorf("from context = %q", got)
	}
}

func TestFullResourceNameExpander(t *testing.T) {
	expand := FullResourceNameExpander()
	ctx := TransformContext{Project: "p", Location: "us-central1", ResourceType: "queues"}

	// short name -> fully qualified
	out, err := expand(map[string]interface{}{"name": "q1", "x": 1}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if out["name"] != "projects/p/locations/us-central1/queues/q1" {
		t.Errorf("expanded name = %q", out["name"])
	}
	if out["x"] != 1 {
		t.Errorf("other fields dropped: %v", out)
	}

	// already-qualified name untouched (idempotent)
	full := "projects/p/locations/us-central1/queues/q1"
	out, _ = expand(map[string]interface{}{"name": full}, ctx)
	if out["name"] != full {
		t.Errorf("qualified name mutated = %q", out["name"])
	}

	// missing name is a no-op
	out, _ = expand(map[string]interface{}{"x": 2}, ctx)
	if _, ok := out["name"]; ok {
		t.Errorf("name injected unexpectedly: %v", out)
	}
}
