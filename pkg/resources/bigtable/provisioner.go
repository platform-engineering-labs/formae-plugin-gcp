// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package bigtable

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// BigtableProvisioner wraps BaseResource to handle Bigtable-specific Create behavior
type BigtableProvisioner struct {
	*base.BaseResource
	resourceTypeAPI string // "instances", "clusters", "tables"
}

// Status routes through base.StatusWithRead so a completed async create comes
// back carrying the resource's properties.
//
// BigtableProvisioner embeds *base.BaseResource and overrode only Create, so it
// inherited the raw BaseResource.Status - which reports success and nothing
// else. UnifiedProvisioner wraps that with a Read for exactly this reason; a
// hand-written provisioner has to do the same or the resource has no properties
// after it is created.
//
// The visible symptom was a reference to a Bigtable instance never resolving:
// with no properties on the instance there is no ".name" to read, so a table
// declared alongside its instance reached the plugin with an unresolved
// reference and failed with "instance is required for nested resources".
func (p *BigtableProvisioner) Status(
	ctx context.Context,
	request *resource.StatusRequest,
) (*resource.StatusResult, error) {
	return base.StatusWithRead(ctx, p.BaseResource, p.Read, request)
}

// Create overrides the base Create to add Bigtable-specific query parameters
func (p *BigtableProvisioner) Create(
	ctx context.Context,
	request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	client, err := transport.NewClient(ctx, p.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport client: %w", err)
	}

	// Parse properties
	var props map[string]interface{}
	if err := json.Unmarshal(request.Properties, &props); err != nil {
		return createBigtableFailureResult(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to parse properties: %v", err)), nil
	}
	// base.Create does this and this provisioner did not, so any property whose
	// value arrived wrapped - which is what a reference to another resource
	// produces - read as empty. A table declared alongside its instance failed
	// with "instance is required for nested resources" even though the instance
	// was right there, which is why bigtable's table schema typed `instance` as
	// a plain String and why the type still has no conformance case.
	props = base.UnwrapValues(props)

	// A forma can wrap a property so it is stored or displayed differently, and
	// base.Create unwraps those before reading anything. This provisioner is
	// hand-written and did not, so a wrapped value read as a plain string came
	// back empty - which surfaced as "instance is required for nested
	// resources" on a table whose instance was in fact declared.
	props = base.UnwrapValues(props)

	// Extract resource name for query parameter
	resourceName := utils.GetString(props, "name")
	if resourceName == "" {
		return createBigtableFailureResult(resource.OperationErrorCodeInvalidRequest,
			"resource name is required"), nil
	}

	// Build path context from config and properties
	cfg := config.FromTargetConfig(request.TargetConfig, p.Config.Deps())
	pathCtx := base.PathContext{
		Project:      cfg.Project,
		ResourceType: p.ResourceConfig.ResourceType,
		ResourceName: resourceName,
	}

	// For nested resources, set parent
	if p.ResourceConfig.ParentResource != nil {
		parentProp := p.ResourceConfig.ParentResource.PropertyName
		if parentProp == "" {
			parentProp = p.ResourceConfig.ParentResource.ParentType
		}
		parent := utils.GetString(props, parentProp)
		if parent == "" && p.ResourceConfig.ParentResource.RequiresParent {
			// Say what was actually received. "instance is required" on its own
			// is unfalsifiable from a CI log - the property may be absent, may
			// be present under another name, or may be a type this code cannot
			// read - and a plugin-side create failure carries no other
			// diagnostic out of an apply.
			return createBigtableFailureResult(resource.OperationErrorCodeInvalidRequest,
				fmt.Sprintf("%s is required for nested resources; got %s from properties %s",
					parentProp, describeValue(props[parentProp]), sortedKeys(props))), nil
		}
		pathCtx.ParentResource = parent
		pathCtx.ParentType = p.ResourceConfig.ParentResource.ParentType

		// For backups (three-level hierarchy), also extract cluster name
		if p.ResourceConfig.ResourceType == "backups" {
			cluster := utils.GetString(props, "cluster")
			if cluster == "" {
				return createBigtableFailureResult(resource.OperationErrorCodeInvalidRequest,
					"cluster is required for backup resources"), nil
			}
			pathCtx.CustomSegments = []string{cluster}
		}
	}

	// Transform request properties
	body := props
	if p.RequestTransformer != nil {
		transformCtx := base.TransformContext{
			Project:      pathCtx.Project,
			ResourceType: pathCtx.ResourceType,
			Operation:    resource.OperationCreate,
		}
		body, err = p.RequestTransformer.Transform(props, transformCtx)
		if err != nil {
			return createBigtableFailureResult(resource.OperationErrorCodeInvalidRequest,
				fmt.Sprintf("failed to transform request: %v", err)), nil
		}
	}

	// Apply request wrapper if configured
	if p.ResourceConfig.RequestWrapper != "" {
		body = map[string]interface{}{
			p.ResourceConfig.RequestWrapper: body,
		}
	}

	// Build URL (without resource name in path for collection URL)
	urlBuilder := base.NewURLBuilder(p.APIConfig, pathCtx)
	url := urlBuilder.CollectionURL()

	// Add Bigtable-specific query parameter for resource ID
	// Format: ?instance_id={name} or ?cluster_id={name} or ?table_id={name}
	// Note: Bigtable Admin API uses snake_case for query parameters
	url, err = transport.AddQueryParams(url, map[string]string{
		bigtableIDParam(p.resourceTypeAPI): resourceName,
	})
	if err != nil {
		return createBigtableFailureResult(resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("failed to build URL: %v", err)), nil
	}

	// Send create request
	response, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "POST",
		URL:    url,
		Body:   body,
	})
	if err != nil {
		transportErr := transport.WrapError(err, "failed to create resource")
		errMsg := fmt.Sprintf("Create failed for %s: %s (URL: %s)", request.ResourceType, transportErr.Message, url)
		return createBigtableFailureResult(transport.ToResourceErrorCode(transportErr.Code),
			errMsg), nil
	}

	// Check if this is a long-running operation
	if operationName, ok := response.Body["name"].(string); ok {
		// If it contains "operations", it's a long-running operation
		if strings.Contains(operationName, "/operations/") {
			// Extract native ID and operation ID for async operations
			nativeID := p.OperationConfig.NativeIDExtractor(response.Body, pathCtx)
			operationID := p.OperationConfig.OperationIDExtractor(response.Body)
			requestID := p.OperationConfig.OperationURLBuilder(pathCtx, operationID)

			return &resource.CreateResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationCreate,
					OperationStatus: resource.OperationStatusInProgress,
					NativeID:        nativeID,
					RequestID:       requestID,
					StatusMessage:   fmt.Sprintf("%s creation in progress", p.ResourceConfig.ResourceType),
				},
			}, nil
		}

		// Otherwise, it's a synchronous response with the created resource
		nativeID := p.OperationConfig.NativeIDExtractor(response.Body, pathCtx)

		// Transform response if transformer provided
		responseBody := response.Body
		if p.ResponseTransformer != nil {
			transformCtx := base.TransformContext{
				Project:      pathCtx.Project,
				ResourceType: pathCtx.ResourceType,
				Operation:    resource.OperationCreate,
			}
			responseBody = p.ResponseTransformer.Transform(response.Body, transformCtx)
		}

		propsJSON, _ := json.Marshal(responseBody)

		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:          resource.OperationCreate,
				NativeID:           nativeID,
				OperationStatus:    resource.OperationStatusSuccess,
				StatusMessage:      "Resource created successfully",
				ResourceProperties: propsJSON,
			},
		}, nil
	}

	return createBigtableFailureResult(resource.OperationErrorCodeUnforeseenError,
		"Invalid response: missing name field"), nil
}

// createBigtableFailureResult creates a failure result
func createBigtableFailureResult(errorCode resource.OperationErrorCode, message string) *resource.CreateResult {
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       errorCode,
			StatusMessage:   message,
		},
	}
}

// newBigtableProvisionerWithBase creates a BigtableProvisioner from a BaseResource
func newBigtableProvisionerWithBase(baseResource *base.BaseResource, resourceTypeAPI string) *BigtableProvisioner {
	return &BigtableProvisioner{
		BaseResource:    baseResource,
		resourceTypeAPI: resourceTypeAPI,
	}
}

// bigtableIDParam names the query parameter that carries a new resource's id.
// The API collection is camelCase while the parameter is snake_case, so
// "materializedViews" has to become "materialized_view_id" - trimming the
// plural alone yielded "materializedView_id", which the API ignored and then
// rejected the create for an empty id.
func bigtableIDParam(resourceTypeAPI string) string {
	var b strings.Builder
	for i, r := range strings.TrimSuffix(resourceTypeAPI, "s") {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r - 'A' + 'a')
			continue
		}
		b.WriteRune(r)
	}
	b.WriteString("_id")
	return b.String()
}

// describeValue renders a property's Go type and value for an error message,
// so "missing" can be told apart from "present but unreadable".
func describeValue(v interface{}) string {
	if v == nil {
		return "<absent>"
	}
	return fmt.Sprintf("%T(%v)", v, v)
}

// sortedKeys lists the property names a request actually carried.
func sortedKeys(props map[string]interface{}) string {
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}
