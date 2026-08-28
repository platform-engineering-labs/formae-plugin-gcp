// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigtable

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// Resource type constants
const (
	InstanceResourceType = "GCP::Bigtable::Instance"
	ClusterResourceType  = "GCP::Bigtable::Cluster"
	TableResourceType    = "GCP::Bigtable::Table"

	AppProfileResourceType       = "GCP::Bigtable::AppProfile"
	LogicalViewResourceType      = "GCP::Bigtable::LogicalView"
	MaterializedViewResourceType = "GCP::Bigtable::MaterializedView"
)

// bigtableRegistry is the unified registry for all Bigtable resources
var bigtableRegistry *base.ResourceRegistry

func NewBigtableProvisioner(cfg *config.Config, resourceType string) (prov.Provisioner, error) {
	if bigtableRegistry == nil {
		return nil, fmt.Errorf("bigtable registry not initialized")
	}

	def, ok := bigtableRegistry.Definitions[resourceType]
	if !ok {
		return nil, fmt.Errorf("no configuration found for resource type: %s", resourceType)
	}

	// Create BaseResource
	baseResource := &base.BaseResource{
		Config:              cfg,
		APIConfig:           BigtableAPI,
		OperationConfig:     BigtableOperations,
		ResourceConfig:      def.ResourceConfig,
		NativeIDConfig:      BigtableNativeID,
		RequestTransformer:  def.RequestTransformer,
		ResponseTransformer: def.ResponseTransformer,
	}

	// Wrap with Bigtable-specific provisioner to handle Create query parameters
	return newBigtableProvisionerWithBase(baseResource, def.ResourceConfig.ResourceType), nil
}

// Wrapper functions to adapt transformers to base interfaces
func wrapInstanceBodyBuilder(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	return instanceBodyBuilderWithContext(props, ctx.Project)
}

func wrapClusterBodyBuilder(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	return clusterBodyBuilder(props, ctx.Project)
}

func wrapTableBodyBuilder(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	return tableBodyBuilder(props)
}

func init() {
	// Create the registry with common Bigtable API configurations
	bigtableRegistry = base.NewResourceRegistry(
		BigtableAPI,
		BigtableOperations,
		BigtableNativeID,
	)

	// Register all Bigtable resources
	err := bigtableRegistry.RegisterAll([]base.ResourceDefinition{
		{
			ResourceType: InstanceResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType:   "instances",
				Scope:          nil, // Instances are project-level resources
				ParentResource: nil, // Top-level resource
				SupportsUpdate: false,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       false,
					FieldName:     "",
					LocationInURL: false,
				},
				RequestWrapper: "", // Instance builder handles wrapping internally due to special structure
			},
			RequestTransformer:  base.RequestTransformerFunc(wrapInstanceBodyBuilder),
			ResponseTransformer: base.ResponseTransformerFunc(instanceResponseTransformer),
		},
		{
			ResourceType: ClusterResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "clusters",
				Scope:        nil,
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "instances",
					PropertyName:   "instance",
					RequiresParent: true,
				},
				SupportsUpdate: false,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       false,
					FieldName:     "",
					LocationInURL: false,
				},
				RequestWrapper: "", // Cluster API doesn't require wrapping
			},
			RequestTransformer:  base.RequestTransformerFunc(wrapClusterBodyBuilder),
			ResponseTransformer: base.ResponseTransformerFunc(clusterResponseTransformer),
		},
		{
			ResourceType: TableResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "tables",
				Scope:        nil,
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "instances",
					PropertyName:   "instance",
					RequiresParent: true,
				},
				SupportsUpdate: false,
				OptimisticLocking: &base.OptimisticLockingConfig{
					Enabled:       false,
					FieldName:     "",
					LocationInURL: false,
				},
				RequestWrapper: "table", // Bigtable API expects payload wrapped in "table"
			},
			RequestTransformer:  base.RequestTransformerFunc(wrapTableBodyBuilder),
			ResponseTransformer: base.AddProjectResponseTransformer,
		},
		// The three types below take their create id as a camelCase query
		// parameter (?appProfileId=), which CreateIDParam sends verbatim. They
		// deliberately do NOT go through BigtableProvisioner: that derives the
		// parameter by trimming a trailing "s" and appending "_id", which gives
		// "appProfile_id" and is not what the API asks for.
		{
			// An app profile is one of the two Bigtable creates that answer with
			// the resource itself rather than an Operation (a table is the
			// other). Treating it as async made base poll an operation id that
			// was never there, against BaseURL + "/" - which answers 404.
			ResourceType:    AppProfileResourceType,
			OperationConfig: BigtableSyncOperations,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "appProfiles",
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "instances",
					PropertyName:   "instance",
					RequiresParent: true,
				},
				CreateIDParam:      "appProfileId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
				// Bigtable refuses to delete an app profile without this, and
				// refuses to update one that changes routing without it either.
				UpdateQueryParams: map[string]string{"ignoreWarnings": "true"},
				DeleteQueryParams: map[string]string{"ignoreWarnings": "true"},
			},
			RequestTransformer: &base.CompositeRequestTransformer{Transformers: []base.RequestTransformer{
				base.DropFields("instance"),
				base.DropFieldsOnUpdate("name"),
			}},
			ResponseTransformer: base.ResponseTransformerFunc(instanceScopedResponseTransformer),
		},
		{
			ResourceType: LogicalViewResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "logicalViews",
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "instances",
					PropertyName:   "instance",
					RequiresParent: true,
				},
				CreateIDParam:      "logicalViewId",
				SupportsUpdate:     true,
				UpdateMaskFromBody: true,
			},
			RequestTransformer: &base.CompositeRequestTransformer{Transformers: []base.RequestTransformer{
				base.DropFields("instance"),
				base.DropFieldsOnUpdate("name"),
			}},
			ResponseTransformer: base.ResponseTransformerFunc(instanceScopedResponseTransformer),
		},
		{
			// ponytail: no update. A materialized view's query is fixed at
			// creation - the API cannot redefine one in place - and
			// deletionProtection is the only other field, so a change replaces.
			ResourceType: MaterializedViewResourceType,
			ResourceConfig: base.ResourceConfig{
				ResourceType: "materializedViews",
				ParentResource: &base.ParentResourceConfig{
					ParentType:     "instances",
					PropertyName:   "instance",
					RequiresParent: true,
				},
				CreateIDParam:  "materializedViewId",
				SupportsUpdate: false,
			},
			RequestTransformer:  base.DropFields("instance"),
			ResponseTransformer: base.ResponseTransformerFunc(instanceScopedResponseTransformer),
		},
	})

	if err != nil {
		panic(err)
	}

	// Override registrations with BigtableProvisioner for proper Create handling
	// BigtableProvisioner adds the required instance_id/cluster_id/table_id query parameters
	registerInstanceWalkingLists()

	bigtableResourceTypes := []string{InstanceResourceType, ClusterResourceType, TableResourceType}
	for _, rt := range bigtableResourceTypes {
		resourceType := rt // capture by value for closure
		registry.Register(
			resourceType,
			[]resource.Operation{
				resource.OperationCreate,
				resource.OperationRead,
				resource.OperationUpdate,
				resource.OperationDelete,
				resource.OperationList,
				resource.OperationCheckStatus,
			},
			func(cfg *config.Config) prov.Provisioner {
				provisioner, _ := NewBigtableProvisioner(cfg, resourceType)
				return provisioner
			},
		)
	}
}

// instanceScopedResponseTransformer puts back what the API leaves in the
// resource path. Bigtable reports only a full name
// ("projects/{p}/instances/{i}/appProfiles/{a}"), so the instance a forma
// declares would otherwise look absent and every sync would plan a change.
func instanceScopedResponseTransformer(
	props map[string]interface{}, _ base.TransformContext,
) map[string]interface{} {
	out := make(map[string]interface{}, len(props)+1)
	for k, v := range props {
		if k == "etag" {
			continue
		}
		out[k] = v
	}
	name, _ := props["name"].(string)
	parts := strings.Split(name, "/")
	// projects/{p}/instances/{i}/{collection}/{name}
	if len(parts) == 6 && parts[0] == "projects" && parts[2] == "instances" {
		out["instance"] = parts[3]
		out["name"] = parts[5]
	}
	return out
}
