// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package servicedirectory

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestPathBuilderByDepth(t *testing.T) {
	cases := []struct {
		name string
		ctx  base.PathContext
		want string
	}{
		{
			name: "namespace collection",
			ctx:  base.PathContext{Project: "p", Location: "eu", ResourceType: "namespaces"},
			want: "/projects/p/locations/eu/namespaces",
		},
		{
			name: "namespace resource",
			ctx:  base.PathContext{Project: "p", Location: "eu", ResourceType: "namespaces", ResourceName: "ns"},
			want: "/projects/p/locations/eu/namespaces/ns",
		},
		{
			name: "service under its namespace",
			ctx: base.PathContext{Project: "p", Location: "eu", ResourceType: "services", ResourceName: "svc",
				ParentType: "namespaces", ParentResource: "ns"},
			want: "/projects/p/locations/eu/namespaces/ns/services/svc",
		},
		{
			name: "endpoint under service and namespace",
			ctx: base.PathContext{Project: "p", Location: "eu", ResourceType: "endpoints", ResourceName: "ep",
				ParentType: "services", ParentResource: "svc", CustomSegments: []string{"ns"}},
			want: "/projects/p/locations/eu/namespaces/ns/services/svc/endpoints/ep",
		},
	}
	for _, tc := range cases {
		if got := serviceDirectoryPathBuilder(tc.ctx); got != tc.want {
			t.Errorf("%s: path = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A nested resource's parents live only in its native ID. Losing them on parse
// would address the location-level collection and 404 on every read.
func TestParseNativeIDRestoresParents(t *testing.T) {
	ctx, err := parseServiceDirectoryNativeID(
		"projects/p/locations/eu/namespaces/ns/services/svc/endpoints/ep")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ctx.ResourceType != "endpoints" || ctx.ResourceName != "ep" {
		t.Errorf("resource = %s/%s", ctx.ResourceType, ctx.ResourceName)
	}
	if ctx.ParentType != "services" || ctx.ParentResource != "svc" {
		t.Errorf("parent = %s/%s", ctx.ParentType, ctx.ParentResource)
	}
	if len(ctx.CustomSegments) != 1 || ctx.CustomSegments[0] != "ns" {
		t.Errorf("grandparent = %v", ctx.CustomSegments)
	}

	svcCtx, err := parseServiceDirectoryNativeID("projects/p/locations/eu/namespaces/ns/services/svc")
	if err != nil {
		t.Fatalf("parse service: %v", err)
	}
	if svcCtx.ParentResource != "ns" || svcCtx.ResourceName != "svc" || len(svcCtx.CustomSegments) != 0 {
		t.Errorf("service ctx = %+v", svcCtx)
	}

	nsCtx, err := parseServiceDirectoryNativeID("projects/p/locations/eu/namespaces/ns")
	if err != nil {
		t.Fatalf("parse namespace: %v", err)
	}
	if nsCtx.ResourceType != "namespaces" || nsCtx.ResourceName != "ns" || nsCtx.ParentType != "" {
		t.Errorf("namespace ctx = %+v", nsCtx)
	}
}

func TestParseNativeIDRejectsGarbage(t *testing.T) {
	for _, id := range []string{
		"",
		"projects/p/locations/eu",
		"projects/p/locations/eu/namespaces/ns/topics/t",
		"projects/p/regions/eu/namespaces/ns",
		"projects/p/locations/eu/namespaces/ns/services/svc/endpoints",
	} {
		if _, err := parseServiceDirectoryNativeID(id); err == nil {
			t.Errorf("expected error for %q", id)
		}
	}
}

// A List item is only ever a full path, and it is the sole source of a native
// ID there - the path context carries no resource name during discovery.
func TestNativeIDPrefersFullPathName(t *testing.T) {
	got := extractServiceDirectoryNativeID(
		map[string]interface{}{"name": "projects/p/locations/eu/namespaces/ns/services/svc"},
		base.PathContext{})
	if got != "projects/p/locations/eu/namespaces/ns/services/svc" {
		t.Errorf("native id = %q", got)
	}
}

func TestNativeIDFallsBackToContext(t *testing.T) {
	got := extractServiceDirectoryNativeID(map[string]interface{}{}, base.PathContext{
		Project: "p", Location: "eu", ResourceType: "endpoints", ResourceName: "ep",
		ParentType: "services", ParentResource: "svc", CustomSegments: []string{"ns"},
	})
	if got != "projects/p/locations/eu/namespaces/ns/services/svc/endpoints/ep" {
		t.Errorf("native id = %q", got)
	}
}

// The API reports a service's namespace only inside its path, so without this
// the namespace a forma declares reads as absent and every sync plans a change.
func TestResponseTransformersLiftParentsOutOfTheName(t *testing.T) {
	svc := serviceResponseTransformer(map[string]interface{}{
		"name": "projects/p/locations/eu/namespaces/ns/services/svc",
	}, base.TransformContext{})
	if svc["name"] != "svc" || svc["namespace"] != "ns" {
		t.Errorf("service = %+v", svc)
	}

	ep := endpointResponseTransformer(map[string]interface{}{
		"name": "projects/p/locations/eu/namespaces/ns/services/svc/endpoints/ep",
		"port": float64(8080),
	}, base.TransformContext{})
	if ep["name"] != "ep" || ep["namespace"] != "ns" || ep["service"] != "svc" {
		t.Errorf("endpoint = %+v", ep)
	}
	// port is a JSON number in Service Directory responses and must stay one:
	// stringifying it would make an already-correct endpoint plan an update.
	if _, ok := ep["port"].(float64); !ok {
		t.Errorf("port = %T, want float64", ep["port"])
	}
}

func TestResponseTransformersLeaveShortNamesAlone(t *testing.T) {
	out := serviceResponseTransformer(map[string]interface{}{"name": "svc"}, base.TransformContext{})
	if out["name"] != "svc" {
		t.Errorf("name = %v", out["name"])
	}
	if _, ok := out["namespace"]; ok {
		t.Errorf("namespace invented from a short name: %+v", out)
	}
}

func TestAllThreeTypesAreRegistered(t *testing.T) {
	for _, rt := range []string{NamespaceResourceType, ServiceResourceType, EndpointResourceType} {
		for _, op := range []resource.Operation{
			resource.OperationCreate, resource.OperationRead, resource.OperationUpdate,
			resource.OperationDelete, resource.OperationList,
		} {
			if !registry.HasProvisioner(rt, op) {
				t.Errorf("%s not registered for %v", rt, op)
			}
		}
	}
}

// Services and endpoints must keep the parent-walking List, not the generic one
// that would ask a namespace-less collection URL. registerParentWalkingLists is
// called from the package init explicitly because Go runs init functions in
// filename order and "list.go" sorts before "resources.go".
func TestParentWalkingListSurvivesRegistration(t *testing.T) {
	for _, rt := range []string{ServiceResourceType, EndpointResourceType} {
		p := registry.Get(rt, resource.OperationList, nil)
		if p == nil {
			t.Fatalf("%s has no List provisioner", rt)
		}
		if _, ok := p.(*parentWalkListProvisioner); !ok {
			t.Errorf("%s List is %T, want *parentWalkListProvisioner", rt, p)
		}
	}
}
