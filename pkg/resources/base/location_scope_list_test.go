// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package base

import "testing"

// A ScopeLocationBased List used to return an empty result the moment Location
// was empty, with no error and no request. That is the worst of the available
// answers: a target that sets only region (the shape /formae:connect writes)
// reported "no resources exist" for every Cloud Run service and GKE cluster in
// the project, silently, with nothing in the log to notice.
//
// The decision now belongs to the path builder, mirroring how the parent block
// below it already defers: a package whose API can list without an explicit
// location - because a region *is* its location, or because it has a "-"
// wildcard - builds a complete URL, and the request goes out. One that cannot
// leaves an empty path segment, and that request is genuinely doomed, so it is
// still skipped rather than 404ing on every discovery cycle.
func TestListPathIsDoomedOnlyWhenASegmentIsEmpty(t *testing.T) {
	cases := []struct {
		name    string
		builder PathBuilderFunc
		ctx     PathContext
		doomed  bool
	}{
		{
			name: "empty location leaves an empty segment",
			builder: func(ctx PathContext) string {
				return "/projects/" + ctx.Project + "/locations/" + ctx.Location + "/services"
			},
			ctx:    PathContext{Project: "p", IsList: true},
			doomed: true,
		},
		{
			name: "builder falls back to region",
			builder: func(ctx PathContext) string {
				location := ctx.Location
				if location == "" {
					location = ctx.Region
				}
				return "/projects/" + ctx.Project + "/locations/" + location + "/services"
			},
			ctx:    PathContext{Project: "p", Region: "us-central1", IsList: true},
			doomed: false,
		},
		{
			name: "builder substitutes the API wildcard",
			builder: func(ctx PathContext) string {
				location := ctx.Location
				if location == "" && ctx.IsList {
					location = "-"
				}
				return "/projects/" + ctx.Project + "/locations/" + location + "/clusters"
			},
			ctx:    PathContext{Project: "p", IsList: true},
			doomed: false,
		},
		{
			name: "an empty project is doomed too, whatever the location",
			builder: func(ctx PathContext) string {
				return "/projects/" + ctx.Project + "/locations/" + ctx.Location + "/services"
			},
			ctx:    PathContext{Location: "us-central1", IsList: true},
			doomed: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			apiConfig := APIConfig{BaseURL: "https://example.googleapis.com/v2", PathBuilder: tc.builder}
			if got := listPathIsDoomed(apiConfig, tc.ctx); got != tc.doomed {
				t.Errorf("listPathIsDoomed = %v, want %v (path %q)",
					got, tc.doomed, tc.builder(tc.ctx))
			}
		})
	}
}

// The scheme's own "//" must not read as an empty segment - only the path is
// examined, never the base URL.
func TestListPathIsDoomedIgnoresTheScheme(t *testing.T) {
	apiConfig := APIConfig{
		BaseURL:     "https://example.googleapis.com/v2",
		PathBuilder: func(ctx PathContext) string { return "/projects/" + ctx.Project + "/services" },
	}
	if listPathIsDoomed(apiConfig, PathContext{Project: "p", IsList: true}) {
		t.Error("a complete path was reported as doomed")
	}
}
