// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package dataform

import (
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestExpandReleaseConfig(t *testing.T) {
	ctx := base.TransformContext{
		Project: "p", Location: "europe-central2", ParentResource: "repo",
	}
	tests := []struct {
		name    string
		props   map[string]interface{}
		ctx     base.TransformContext
		want    interface{}
		wantErr bool
	}{
		{
			name:  "a short name is expanded to the full path",
			props: map[string]interface{}{"releaseConfig": "rc"},
			ctx:   ctx,
			want:  "projects/p/locations/europe-central2/repositories/repo/releaseConfigs/rc",
		},
		{
			name:  "a full path written out by hand is left alone",
			props: map[string]interface{}{"releaseConfig": "projects/other/locations/l/repositories/r/releaseConfigs/rc"},
			ctx:   ctx,
			want:  "projects/other/locations/l/repositories/r/releaseConfigs/rc",
		},
		{
			name:  "the region stands in for a missing location",
			props: map[string]interface{}{"releaseConfig": "rc"},
			ctx: base.TransformContext{
				Project: "p", Region: "europe-central2", ParentResource: "repo",
			},
			want: "projects/p/locations/europe-central2/repositories/repo/releaseConfigs/rc",
		},
		{
			name:  "no release config at all is left alone",
			props: map[string]interface{}{"name": "wc"},
			ctx:   ctx,
			want:  nil,
		},
		{
			// Better to fail loudly than to send a path with an empty segment,
			// which the API would store verbatim.
			name:    "a missing repository is an error, not a malformed path",
			props:   map[string]interface{}{"releaseConfig": "rc"},
			ctx:     base.TransformContext{Project: "p", Location: "europe-central2"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandReleaseConfig(tt.props, tt.ctx)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got["releaseConfig"] != tt.want {
				t.Errorf("got %#v, want %#v", got["releaseConfig"], tt.want)
			}
		})
	}
}

func TestWorkflowConfigRequestTransformer(t *testing.T) {
	tests := []struct {
		name  string
		props map[string]interface{}
		op    resource.Operation
		want  map[string]interface{}
	}{
		{
			name: "create expands the reference, drops the repository, keeps the rest",
			props: map[string]interface{}{
				"name":             "wc",
				"repository":       "repo",
				"releaseConfig":    "rc",
				"disabled":         true,
				"invocationConfig": map[string]interface{}{"serviceAccount": "sa@p.iam.gserviceaccount.com"},
			},
			op: resource.OperationCreate,
			want: map[string]interface{}{
				"name":             "wc",
				"releaseConfig":    "projects/p/locations/europe-central2/repositories/repo/releaseConfigs/rc",
				"disabled":         true,
				"invocationConfig": map[string]interface{}{"serviceAccount": "sa@p.iam.gserviceaccount.com"},
			},
		},
		{
			// The API refuses a mask naming invocation_config whatever the
			// value, so it must leave the body a reconcile PATCH sends - while
			// releaseConfig, which IS mutable, stays and stays expanded.
			name: "update drops the name and the immutable invocation config",
			props: map[string]interface{}{
				"name":             "wc",
				"repository":       "repo",
				"releaseConfig":    "rc",
				"timeZone":         "UTC",
				"disabled":         true,
				"invocationConfig": map[string]interface{}{"serviceAccount": "sa@p.iam.gserviceaccount.com"},
			},
			op: resource.OperationUpdate,
			want: map[string]interface{}{
				"releaseConfig": "projects/p/locations/europe-central2/repositories/repo/releaseConfigs/rc",
				"timeZone":      "UTC",
				"disabled":      true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := workflowConfigRequestTransformer.Transform(tt.props, base.TransformContext{
				Project: "p", Location: "europe-central2",
				ParentResource: "repo", Operation: tt.op,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %#v\nwant %#v", got, tt.want)
			}
		})
	}
}

func TestWorkflowConfigResponseTransformer(t *testing.T) {
	// The exact shape a live create answered with.
	in := map[string]interface{}{
		"name":          "projects/p/locations/europe-central2/repositories/repo/workflowConfigs/wc",
		"releaseConfig": "projects/p/locations/europe-central2/repositories/repo/releaseConfigs/rc",
		"invocationConfig": map[string]interface{}{
			"serviceAccount": "sa@p.iam.gserviceaccount.com",
			"queryPriority":  "QUERY_PRIORITY_UNSPECIFIED",
		},
		"createTime":       "2026-09-03T15:09:53.884184773Z",
		"updateTime":       "2026-09-03T15:09:53.884184773Z",
		"internalMetadata": `{"unique_ccfe_id":"5d55ddcb"}`,
		"recentScheduledExecutionRecords": []interface{}{
			map[string]interface{}{"executionTime": "2026-09-03T15:00:00Z"},
		},
	}
	want := map[string]interface{}{
		"name":          "wc",
		"repository":    "repo",
		"releaseConfig": "rc",
		"invocationConfig": map[string]interface{}{
			"serviceAccount": "sa@p.iam.gserviceaccount.com",
		},
	}
	got := workflowConfigResponseTransformer.Transform(in, base.TransformContext{})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
}

// A declared query priority is not a placeholder and must survive.
func TestWorkflowConfigResponseKeepsRealQueryPriority(t *testing.T) {
	got := workflowConfigResponseTransformer.Transform(map[string]interface{}{
		"name": "projects/p/locations/europe-central2/repositories/repo/workflowConfigs/wc",
		"invocationConfig": map[string]interface{}{
			"queryPriority": "BATCH",
		},
	}, base.TransformContext{})
	ic, ok := got["invocationConfig"].(map[string]interface{})
	if !ok || ic["queryPriority"] != "BATCH" {
		t.Errorf("got %#v, want queryPriority BATCH", got["invocationConfig"])
	}
}

// releaseConfig is mutable, so a permanent disagreement between the declared
// short name and the stored full path would make every reconcile plan an update
// that changes nothing. Expansion and shortening have to be exact inverses.
func TestReleaseConfigReferenceRoundTrips(t *testing.T) {
	const declared = "formae-test-df-wcrc-1234"

	sent, err := workflowConfigRequestTransformer.Transform(
		map[string]interface{}{
			"name":          "wc",
			"repository":    "repo",
			"releaseConfig": declared,
		},
		base.TransformContext{
			Project: "p", Location: "europe-central2",
			ParentResource: "repo", Operation: resource.OperationCreate,
		})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	full := "projects/p/locations/europe-central2/repositories/repo/releaseConfigs/" + declared
	if sent["releaseConfig"] != full {
		t.Fatalf("request sent %v, want %q", sent["releaseConfig"], full)
	}

	read := workflowConfigResponseTransformer.Transform(map[string]interface{}{
		"name":          "projects/p/locations/europe-central2/repositories/repo/workflowConfigs/wc",
		"releaseConfig": full,
	}, base.TransformContext{})
	if read["releaseConfig"] != declared {
		t.Errorf("read reported %v, want %q", read["releaseConfig"], declared)
	}
}
