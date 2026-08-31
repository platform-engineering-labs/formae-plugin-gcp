// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package spanner

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
			name: "instance collection",
			ctx:  base.PathContext{Project: "p", ResourceType: "instances"},
			want: "/projects/p/instances",
		},
		{
			name: "instance resource",
			ctx:  base.PathContext{Project: "p", ResourceType: "instances", ResourceName: "i"},
			want: "/projects/p/instances/i",
		},
		{
			name: "database under its instance",
			ctx: base.PathContext{Project: "p", ResourceType: "databases", ResourceName: "d",
				ParentType: "instances", ParentResource: "i"},
			want: "/projects/p/instances/i/databases/d",
		},
		{
			name: "backup schedule under database and instance",
			ctx: base.PathContext{Project: "p", ResourceType: "backupSchedules", ResourceName: "b",
				ParentType: "databases", ParentResource: "d", CustomSegments: []string{"i"}},
			want: "/projects/p/instances/i/databases/d/backupSchedules/b",
		},
	}
	for _, tc := range cases {
		if got := spannerPathBuilder(tc.ctx); got != tc.want {
			t.Errorf("%s: path = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A nested resource's parents live only in its native ID. Losing them on parse
// would address the project-level collection and 404 on every read.
func TestParseNativeIDRestoresParents(t *testing.T) {
	ctx, err := parseSpannerNativeID("projects/p/instances/i/databases/d/backupSchedules/b")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ctx.ResourceType != "backupSchedules" || ctx.ResourceName != "b" {
		t.Errorf("resource = %s/%s", ctx.ResourceType, ctx.ResourceName)
	}
	if ctx.ParentType != "databases" || ctx.ParentResource != "d" {
		t.Errorf("parent = %s/%s", ctx.ParentType, ctx.ParentResource)
	}
	if len(ctx.CustomSegments) != 1 || ctx.CustomSegments[0] != "i" {
		t.Errorf("grandparent = %v", ctx.CustomSegments)
	}

	dbCtx, err := parseSpannerNativeID("projects/p/instances/i/databases/d")
	if err != nil {
		t.Fatalf("parse database: %v", err)
	}
	if dbCtx.ParentResource != "i" || dbCtx.ResourceName != "d" || len(dbCtx.CustomSegments) != 0 {
		t.Errorf("database ctx = %+v", dbCtx)
	}

	instCtx, err := parseSpannerNativeID("projects/p/instances/i")
	if err != nil {
		t.Fatalf("parse instance: %v", err)
	}
	if instCtx.ResourceType != "instances" || instCtx.ResourceName != "i" || instCtx.ParentType != "" {
		t.Errorf("instance ctx = %+v", instCtx)
	}
}

func TestParseNativeIDRejectsGarbage(t *testing.T) {
	for _, id := range []string{
		"",
		"projects/p",
		"projects/p/instances/i/tables/t",
		"projects/p/locations/eu/instances/i",
		"projects/p/instances/i/databases/d/backups/b",
	} {
		if _, err := parseSpannerNativeID(id); err == nil {
			t.Errorf("expected error for %q", id)
		}
	}
}

// An async create answers with an Operation, and Spanner hangs operations off
// the resource being created - the native ID is the part in front.
func TestNativeIDStripsTheOperationSuffix(t *testing.T) {
	got := extractSpannerNativeID(map[string]interface{}{
		"name": "projects/p/instances/i/databases/d/operations/_auto_op_1",
	}, base.PathContext{})
	if got != "projects/p/instances/i/databases/d" {
		t.Errorf("native id = %q", got)
	}
}

func TestNativeIDFallsBackToContext(t *testing.T) {
	got := extractSpannerNativeID(map[string]interface{}{}, base.PathContext{
		Project: "p", ResourceType: "backupSchedules", ResourceName: "b",
		ParentType: "databases", ParentResource: "d", CustomSegments: []string{"i"},
	})
	if got != "projects/p/instances/i/databases/d/backupSchedules/b" {
		t.Errorf("native id = %q", got)
	}
}

// Create puts the id beside the instance rather than in a query parameter,
// which is why this is a transformer and not CreateIDParam + RequestWrapper.
func TestInstanceCreateEnvelope(t *testing.T) {
	body, err := instanceRequestTransformer(map[string]interface{}{
		"name":            "my-instance",
		"config":          "regional-europe-central2",
		"displayName":     "Formae",
		"processingUnits": 100,
	}, base.TransformContext{Project: "p", Operation: resource.OperationCreate})
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	if body["instanceId"] != "my-instance" {
		t.Errorf("instanceId = %v", body["instanceId"])
	}
	inst, ok := body["instance"].(map[string]interface{})
	if !ok {
		t.Fatalf("instance = %T", body["instance"])
	}
	if _, present := inst["name"]; present {
		t.Errorf("name must not be repeated inside the instance: %+v", inst)
	}
	// A forma names the config by its id; the API wants the full path.
	if inst["config"] != "projects/p/instanceConfigs/regional-europe-central2" {
		t.Errorf("config = %v", inst["config"])
	}
	if _, present := body["fieldMask"]; present {
		t.Errorf("create must not carry a field mask: %+v", body)
	}
}

// A patch names the instance by its full path, and its mask must list only the
// mutable fields - "config" in the mask is rejected outright.
func TestInstanceUpdateEnvelopeMasksOnlyMutableFields(t *testing.T) {
	body, err := instanceRequestTransformer(map[string]interface{}{
		"name":            "my-instance",
		"config":          "regional-europe-central2",
		"displayName":     "Formae",
		"processingUnits": 100,
		"labels":          map[string]interface{}{"a": "b"},
	}, base.TransformContext{Project: "p", Operation: resource.OperationUpdate})
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	inst := body["instance"].(map[string]interface{})
	if inst["name"] != "projects/p/instances/my-instance" {
		t.Errorf("instance.name = %v", inst["name"])
	}
	mask, _ := body["fieldMask"].(string)
	if mask != "displayName,labels,processingUnits" {
		t.Errorf("fieldMask = %q", mask)
	}
}

// An already-qualified config must not be qualified twice.
func TestInstanceConfigIsQualifiedOnlyOnce(t *testing.T) {
	full := "projects/other/instanceConfigs/nam3"
	if got := qualifyInstanceConfig(full, "p"); got != full {
		t.Errorf("config = %v", got)
	}
}

func TestInstanceResponseShortensBothPaths(t *testing.T) {
	out := instanceResponseTransformer(map[string]interface{}{
		"name":   "projects/p/instances/my-instance",
		"config": "projects/p/instanceConfigs/regional-europe-central2",
	}, base.TransformContext{})
	if out["name"] != "my-instance" {
		t.Errorf("name = %v", out["name"])
	}
	// Keeping the qualified config would put the project id in every forma.
	if out["config"] != "regional-europe-central2" {
		t.Errorf("config = %v", out["config"])
	}
}

// Spanner has no name field on create: the id goes into a DDL statement, and
// an id containing a hyphen is rejected unquoted in either dialect.
func TestDatabaseCreateStatementQuotesForTheDialect(t *testing.T) {
	body, err := databaseRequestTransformer(map[string]interface{}{
		"name": "my-db", "instance": "i",
	}, base.TransformContext{})
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	if body["createStatement"] != "CREATE DATABASE `my-db`" {
		t.Errorf("googlesql statement = %v", body["createStatement"])
	}
	if _, present := body["instance"]; present {
		t.Errorf("instance addresses the database, it is not a body field: %+v", body)
	}

	body, err = databaseRequestTransformer(map[string]interface{}{
		"name": "my-db", "databaseDialect": "POSTGRESQL",
	}, base.TransformContext{})
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	if body["createStatement"] != `CREATE DATABASE "my-db"` {
		t.Errorf("postgres statement = %v", body["createStatement"])
	}
	if body["databaseDialect"] != "POSTGRESQL" {
		t.Errorf("dialect dropped: %+v", body)
	}
}

func TestDatabaseCreateNeedsAName(t *testing.T) {
	if _, err := databaseRequestTransformer(map[string]interface{}{}, base.TransformContext{}); err == nil {
		t.Error("expected an error for a database with no name")
	}
}

// The API reports a database's instance only inside its path, so without this
// the instance a forma declares reads as absent and every sync plans a change.
func TestResponseTransformersLiftParentsOutOfTheName(t *testing.T) {
	db := databaseResponseTransformer(map[string]interface{}{
		"name": "projects/p/instances/i/databases/d",
	}, base.TransformContext{})
	if db["name"] != "d" || db["instance"] != "i" {
		t.Errorf("database = %+v", db)
	}

	bs := backupScheduleResponseTransformer(map[string]interface{}{
		"name": "projects/p/instances/i/databases/d/backupSchedules/b",
	}, base.TransformContext{})
	if bs["name"] != "b" || bs["instance"] != "i" || bs["database"] != "d" {
		t.Errorf("backup schedule = %+v", bs)
	}
}

func TestResponseTransformersLeaveShortNamesAlone(t *testing.T) {
	out := databaseResponseTransformer(map[string]interface{}{"name": "d"}, base.TransformContext{})
	if out["name"] != "d" {
		t.Errorf("name = %v", out["name"])
	}
	if _, ok := out["instance"]; ok {
		t.Errorf("instance invented from a short name: %+v", out)
	}
}

func TestAllThreeTypesAreRegistered(t *testing.T) {
	for _, rt := range []string{InstanceResourceType, DatabaseResourceType, BackupScheduleResourceType} {
		for _, op := range []resource.Operation{
			resource.OperationCreate, resource.OperationRead,
			resource.OperationDelete, resource.OperationList,
		} {
			if !registry.HasProvisioner(rt, op) {
				t.Errorf("%s not registered for %v", rt, op)
			}
		}
	}
}

// Databases and backup schedules must keep the parent-walking List, not the
// generic one that would ask an instance-less collection URL.
// registerParentWalkingLists is called from the package init explicitly because
// Go runs init functions in filename order and "list.go" sorts before
// "resources.go".
func TestParentWalkingListSurvivesRegistration(t *testing.T) {
	for _, rt := range []string{DatabaseResourceType, BackupScheduleResourceType} {
		p := registry.Get(rt, resource.OperationList, nil)
		if p == nil {
			t.Fatalf("%s has no List provisioner", rt)
		}
		if _, ok := p.(*parentWalkListProvisioner); !ok {
			t.Errorf("%s List is %T, want *parentWalkListProvisioner", rt, p)
		}
	}
}
