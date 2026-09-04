// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package spanner

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const (
	InstanceResourceType       = "GCP::Spanner::Instance"
	InstanceConfigResourceType = "GCP::Spanner::InstanceConfig"
	DatabaseResourceType       = "GCP::Spanner::Database"
	BackupScheduleResourceType = "GCP::Spanner::BackupSchedule"
)

// instanceMutableFields are the only fields spanner.instances.patch accepts in
// its fieldMask. Anything else - config above all, which pins the instance's
// region - is fixed at creation.
var instanceMutableFields = []string{"displayName", "labels", "processingUnits", "nodeCount", "edition"}

var spannerRegistry *base.ResourceRegistry

func init() {
	spannerRegistry = base.NewResourceRegistry(SpannerAPI, SpannerOperations, SpannerNativeID)

	err := spannerRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: InstanceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "instances",
				Scope:          &base.ScopeConfig{Type: base.ScopeProjectLevel},
				SupportsUpdate: true,
				// The field mask goes in the body as "fieldMask", not in the
				// query string, so the transformer builds it.
				UpdateMethod: base.UpdateMethodPatch,
			},
			RequestTransformer:  base.RequestTransformerFunc(instanceRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(instanceResponseTransformer),
		},
		{
			// A user-managed instance configuration: project-scoped like an
			// instance, and free to hold - nothing is provisioned or billed
			// until an instance is created against it.
			//
			// RequestWrapper, CreateIDParam and UpdateMaskFromBody are all
			// deliberately unset; instanceConfigRequestTransformer explains
			// which envelope each one would have produced and why none of them
			// is the envelope Spanner accepts.
			ResourceType: InstanceConfigResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "instanceConfigs",
				Scope:          &base.ScopeConfig{Type: base.ScopeProjectLevel},
				SupportsUpdate: true,
				UpdateMethod:   base.UpdateMethodPatch,
			},
			RequestTransformer:  base.RequestTransformerFunc(instanceConfigRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(instanceConfigResponseTransformer),
		},
		{
			// ponytail: no update. enableDropProtection is the only patchable
			// field on a database; everything else a forma can say is fixed at
			// creation, so a change is correctly a replace.
			ResourceType: DatabaseResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "databases",
				Scope:        &base.ScopeConfig{Type: base.ScopeProjectLevel},
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "instances",
					PropertyName:   "instance",
					RequiresParent: true,
				},
			},
			Operations: []resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			RequestTransformer:  base.RequestTransformerFunc(databaseRequestTransformer),
			ResponseTransformer: base.ResponseTransformerFunc(databaseResponseTransformer),
		},
		{
			ResourceType:    BackupScheduleResourceType,
			OperationConfig: SpannerSyncOperations,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "backupSchedules",
				Scope:        &base.ScopeConfig{Type: base.ScopeProjectLevel},
				ParentResource: &base.ParentResourceConfig{
					ParentType:              "databases",
					PropertyName:            "database",
					RequiresParent:          true,
					GrandParentType:         "instances",
					GrandParentPropertyName: "instance",
				},
				CreateIDParam:  "backupScheduleId",
				SupportsUpdate: true,
				// The update mask is fixed rather than computed from the body.
				// backupSchedules.patch accepts exactly three paths - it answers
				// "Field mask contains invalid path(s): spec, full_backup_spec.
				// Allowed paths are: encryption_config, retention_duration,
				// spec.cron_spec.text" - and a mask reaching one field *inside*
				// spec is not something UpdateMaskFromBody can express, since
				// that lists the body's top-level fields. encryption_config is
				// left out: it is a provider default nobody declares, so
				// masking it would rewrite a server value on every update.
				UpdateQueryParams: map[string]string{
					"updateMask": "retentionDuration,spec.cronSpec.text",
				},
			},
			RequestTransformer: &base.CompositeRequestTransformer{Transformers: []base.RequestTransformer{
				// "instance" and "database" address the schedule rather than
				// describing it; the API rejects either as an unknown body
				// field. "name" stays on create - the engine reads the id
				// (?backupScheduleId=) from it - and goes on update, where it
				// would land in the field mask.
				base.DropFields("instance", "database"),
				base.DropFieldsOnUpdate("name"),
			}},
			ResponseTransformer: base.ResponseTransformerFunc(backupScheduleResponseTransformer),
		},
	})
	if err != nil {
		panic(err)
	}

	registerParentWalkingLists()
}

// instanceRequestTransformer builds the two envelopes spanner.instances uses.
// Neither is expressible with RequestWrapper + CreateIDParam: the id sits in
// the body *beside* the instance rather than in a query parameter, and the
// update mask is a body field rather than a query parameter.
//
//	create: {"instanceId": "x", "instance": {...}}
//	patch:  {"instance": {"name": "<full path>", ...}, "fieldMask": "a,b"}
func instanceRequestTransformer(
	props map[string]interface{}, ctx base.TransformContext,
) (map[string]interface{}, error) {
	name, _ := props["name"].(string)

	instance := make(map[string]interface{}, len(props))
	for k, v := range props {
		if k == "name" {
			continue // carried by instanceId on create, by the full path on patch
		}
		if k == "config" {
			v = qualifyInstanceConfig(v, ctx.Project)
		}
		instance[k] = v
	}

	if ctx.Operation != resource.OperationUpdate {
		return map[string]interface{}{"instanceId": name, "instance": instance}, nil
	}

	// A patch names the instance by its full path and must mask only the
	// mutable fields - "config" in the mask is rejected outright.
	instance["name"] = fmt.Sprintf("projects/%s/instances/%s", ctx.Project, name)
	mask := make([]string, 0, len(instanceMutableFields))
	for _, f := range instanceMutableFields {
		if _, ok := instance[f]; ok {
			mask = append(mask, f)
		}
	}
	if len(mask) == 0 {
		return nil, fmt.Errorf("no mutable spanner instance fields to update")
	}
	return map[string]interface{}{"instance": instance, "fieldMask": strings.Join(mask, ",")}, nil
}

// instanceResponseTransformer shortens the two full paths an instance reports,
// so the read state matches what a forma declares: "name" to its last segment,
// and "config" to the instance-config id ("regional-europe-central2"). Keeping
// the qualified config would put the project id in every forma and make one
// unportable between targets.
func instanceResponseTransformer(
	props map[string]interface{}, _ base.TransformContext,
) map[string]interface{} {
	out := copyProps(props)
	if name, ok := props["name"].(string); ok {
		out["name"] = lastSegment(name)
	}
	if cfg, ok := props["config"].(string); ok {
		out["config"] = lastSegment(cfg)
	}
	return out
}

// qualifyInstanceConfig is the inverse: a forma names an instance config by its
// id and the API wants the full path.
func qualifyInstanceConfig(v interface{}, project string) interface{} {
	cfg, ok := v.(string)
	if !ok || cfg == "" || strings.Contains(cfg, "/") {
		return v
	}
	return fmt.Sprintf("projects/%s/instanceConfigs/%s", project, cfg)
}

// databaseRequestTransformer turns the declared id into the DDL statement the
// API creates a database from - Spanner has no "name" field on create. The
// quoting follows the dialect: GoogleSQL quotes identifiers with backticks,
// PostgreSQL with double quotes, and a database id containing a hyphen is
// rejected unquoted in either.
func databaseRequestTransformer(
	props map[string]interface{}, _ base.TransformContext,
) (map[string]interface{}, error) {
	name, _ := props["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("spanner database requires a name")
	}

	dialect, _ := props["databaseDialect"].(string)
	quoted := "`" + name + "`"
	if dialect == "POSTGRESQL" {
		quoted = `"` + name + `"`
	}

	body := map[string]interface{}{"createStatement": "CREATE DATABASE " + quoted}
	for _, k := range []string{"databaseDialect", "encryptionConfig", "extraStatements"} {
		if v, ok := props[k]; ok {
			body[k] = v
		}
	}
	return body, nil
}

// databaseResponseTransformer puts back what the API leaves in the resource
// path. A database reports only its full name, so the instance a forma declares
// would otherwise look absent and every sync would plan a change.
func databaseResponseTransformer(
	props map[string]interface{}, _ base.TransformContext,
) map[string]interface{} {
	out := copyProps(props)
	// projects/{p}/instances/{i}/databases/{d}
	if parts := pathParts(props, 6); parts != nil {
		out["instance"] = parts[3]
		out["name"] = parts[5]
	}
	return out
}

// backupScheduleResponseTransformer does the same two levels deeper: a
// schedule's instance and database both live only in its path.
func backupScheduleResponseTransformer(
	props map[string]interface{}, _ base.TransformContext,
) map[string]interface{} {
	out := copyProps(props)
	// projects/{p}/instances/{i}/databases/{d}/backupSchedules/{b}
	if parts := pathParts(props, 8); parts != nil && parts[6] == "backupSchedules" {
		out["instance"] = parts[3]
		out["database"] = parts[5]
		out["name"] = parts[7]
	}
	return out
}

func copyProps(props map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(props)+2)
	for k, v := range props {
		out[k] = v
	}
	return out
}

func lastSegment(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// pathParts splits a response's full-path "name" when it has exactly n
// segments, and returns nil otherwise - a short name has already been
// transformed, or the response is not the shape this transformer handles.
func pathParts(props map[string]interface{}, n int) []string {
	name, _ := props["name"].(string)
	if name == "" {
		return nil
	}
	parts := strings.Split(name, "/")
	if len(parts) != n || parts[0] != "projects" || parts[2] != "instances" || parts[4] != "databases" {
		return nil
	}
	return parts
}
