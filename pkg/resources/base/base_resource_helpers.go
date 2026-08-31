// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package base

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// buildUpdateMask returns a comma-separated, sorted list of the body's top-level
// field names for use as a GCP "updateMask" query parameter, or "" when masking
// is disabled or the body is empty.
func buildUpdateMask(enabled bool, body map[string]interface{}) string {
	if !enabled || len(body) == 0 {
		return ""
	}
	fields := make([]string, 0, len(body))
	for k := range body {
		fields = append(fields, k)
	}
	sort.Strings(fields)
	return strings.Join(fields, ",")
}

// buildPathContext builds a PathContext from target config and properties
func (b *BaseResource) buildPathContext(targetConfig json.RawMessage, props map[string]interface{}) PathContext {
	cfg := config.PathFromTargetConfig(targetConfig)
	// Use explicit Location only - no fallback to Region
	ctx := PathContext{
		Project:      cfg.Project,
		Region:       cfg.Region,
		Zone:         cfg.Zone,
		Location:     cfg.Location, // Container/CloudRun use location (no Region fallback)
		ResourceType: b.ResourceConfig.ResourceType,
	}

	// Respect resource scope - clear region/zone if not applicable
	if b.ResourceConfig.Scope != nil {
		switch b.ResourceConfig.Scope.Type {
		case ScopeGlobal:
			// Global resources don't use region or zone
			ctx.Region = ""
			ctx.Zone = ""
		case ScopeRegional:
			// Regional resources don't use zone
			ctx.Zone = ""
		case ScopeZonal:
			// Zonal resources use both region and zone
			// Keep both as-is
		}
	}

	// Extract name from properties
	if name, ok := props["name"].(string); ok {
		ctx.ResourceName = name
	}

	// Extract parent resource for nested resources
	if b.ResourceConfig.ParentResource != nil && b.ResourceConfig.ParentResource.RequiresParent {
		// Use PropertyName if specified, otherwise fall back to ParentType
		propName := b.ResourceConfig.ParentResource.PropertyName
		if propName == "" {
			propName = b.ResourceConfig.ParentResource.ParentType
		}
		if parent, ok := props[propName].(string); ok {
			// A parent identified by two properties - a Storage object ACL hangs
			// off a bucket AND an object - is carried as "{first}/{second}",
			// which is the form the Storage path builder and native ID expect.
			if second := b.ResourceConfig.ParentResource.SecondPropertyName; second != "" {
				if secondValue, ok := props[second].(string); ok && secondValue != "" {
					parent = parent + "/" + secondValue
				}
			}
			ctx.ParentResource = parent
			ctx.ParentType = b.ResourceConfig.ParentResource.ParentType
		}

		// A resource nested two levels deep (namespaces > services > endpoints)
		// carries its grandparent in CustomSegments[0].
		if gpType := b.ResourceConfig.ParentResource.GrandParentType; gpType != "" {
			gpProp := b.ResourceConfig.ParentResource.GrandParentPropertyName
			if gpProp == "" {
				gpProp = gpType
			}
			if gp, ok := props[gpProp].(string); ok && gp != "" {
				ctx.CustomSegments = []string{gp}
			}
		}
	}

	// Extract location if specified in properties (overrides target)
	if location, ok := props["location"].(string); ok {
		ctx.Location = location
		ctx.Region = location
	}

	// Extract zone if specified in properties (overrides target) - for Compute Engine
	if zone, ok := props["zone"].(string); ok {
		ctx.Zone = zone
	}

	return ctx
}

// fillPathContextFromTarget fills missing fields in PathContext from target config
func (b *BaseResource) fillPathContextFromTarget(targetConfig json.RawMessage, ctx *PathContext) {
	cfg := config.PathFromTargetConfig(targetConfig)
	if ctx.Project == "" {
		ctx.Project = cfg.Project
	}

	// Use explicit Location only - no fallback to Region for location-based APIs

	// Respect resource scope when filling in region/zone
	if b.ResourceConfig.Scope != nil {
		switch b.ResourceConfig.Scope.Type {
		case ScopeGlobal:
			// Global resources don't use region or zone - don't fill them in
			ctx.Region = ""
			ctx.Zone = ""
			ctx.Location = ""
		case ScopeRegional:
			// Regional resources use region but not zone
			if ctx.Region == "" {
				ctx.Region = cfg.Region
			}
			ctx.Zone = ""
		case ScopeZonal:
			// Zonal resources use both region and zone
			if ctx.Region == "" {
				ctx.Region = cfg.Region
			}
			if ctx.Zone == "" {
				ctx.Zone = cfg.Zone
			}
		case ScopeLocationBased:
			// Location-based resources (Container/CloudRun) use location only
			// No fallback to Region - location must be explicitly set
			if ctx.Location == "" {
				ctx.Location = cfg.Location
			}
		default:
			// If scope type is not set, use legacy behavior
			if ctx.Region == "" {
				ctx.Region = cfg.Region
			}
			if ctx.Zone == "" {
				ctx.Zone = cfg.Zone
			}
			if ctx.Location == "" {
				ctx.Location = cfg.Location
			}
		}
	} else {
		// If no scope config, use legacy behavior
		if ctx.Region == "" {
			ctx.Region = cfg.Region
		}
		if ctx.Zone == "" {
			ctx.Zone = cfg.Zone
		}
		if ctx.Location == "" {
			ctx.Location = cfg.Location
		}
	}
}

// FillPathContextFromTarget is the public version of fillPathContextFromTarget
// It fills missing fields in PathContext from target config
func (b *BaseResource) FillPathContextFromTarget(targetConfig json.RawMessage, ctx *PathContext) {
	b.fillPathContextFromTarget(targetConfig, ctx)
}

// buildTransformContext builds a TransformContext for transformers
func (b *BaseResource) buildTransformContext(pathCtx PathContext, operation resource.Operation) TransformContext {
	return TransformContext{
		Project:      pathCtx.Project,
		Region:       pathCtx.Region,
		Zone:         pathCtx.Zone,
		Location:     pathCtx.Location,
		ResourceType: pathCtx.ResourceType,
		Operation:    operation,

		ParentResource: pathCtx.ParentResource,
		ParentType:     pathCtx.ParentType,
	}
}

// handleSynchronousCreate handles synchronous create operations
func (b *BaseResource) handleSynchronousCreate(
	ctx context.Context,
	request *resource.CreateRequest,
	responseBody map[string]interface{},
	pathCtx PathContext,
) (*resource.CreateResult, error) {
	nativeID := b.OperationConfig.NativeIDExtractor(responseBody, pathCtx)

	// Transform response if configured
	apiResponse := responseBody
	if b.ResponseTransformer != nil {
		transformCtx := b.buildTransformContext(pathCtx, resource.OperationCreate)
		apiResponse = b.ResponseTransformer.Transform(apiResponse, transformCtx)
	}

	// Marshal properties
	propsJSON, err := json.Marshal(apiResponse)
	if err != nil {
		return b.createFailureResult(resource.OperationErrorCodeServiceInternalError,
			fmt.Sprintf("failed to marshal properties: %v", err)), nil
	}

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           nativeID,
			StatusMessage:      "Resource created successfully",
			ResourceProperties: json.RawMessage(propsJSON),
		},
	}, nil
}

// performUpdate performs a standard update operation
func (b *BaseResource) performUpdate(
	ctx context.Context,
	client *transport.Client,
	request *resource.UpdateRequest,
	props map[string]interface{},
	pathCtx PathContext,
) (*resource.UpdateResult, error) {
	// Transform request properties
	body := props
	var err error
	if b.RequestTransformer != nil {
		transformCtx := b.buildTransformContext(pathCtx, resource.OperationUpdate)
		body, err = b.RequestTransformer.Transform(props, transformCtx)
		if err != nil {
			return b.updateFailureResult(request.NativeID,
				resource.OperationErrorCodeInvalidRequest,
				fmt.Sprintf("failed to transform request: %v", err)), nil
		}
	}

	// Compute an updateMask from the (unwrapped) body fields if configured.
	updateMask := buildUpdateMask(b.ResourceConfig.UpdateMaskFromBody, body)

	// Apply request wrapper if configured
	if b.ResourceConfig.RequestWrapper != "" {
		body = map[string]interface{}{
			b.ResourceConfig.RequestWrapper: body,
		}
	}

	// Build URL using just the resource name (not the full native ID)
	urlBuilder := NewURLBuilder(b.APIConfig, pathCtx)
	url := urlBuilder.ResourceURL(pathCtx.ResourceName)

	// Use configured HTTP method (PATCH or PUT)
	httpMethod := b.ResourceConfig.GetUpdateMethod()

	// Add any configured update query parameters
	if len(b.ResourceConfig.UpdateQueryParams) > 0 {
		url, _ = transport.AddQueryParams(url, b.ResourceConfig.UpdateQueryParams)
	}
	if updateMask != "" {
		url, _ = transport.AddQueryParam(url, "updateMask", updateMask)
	}

	response, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: httpMethod,
		URL:    url,
		Body:   body,
	})
	if err != nil {
		transportErr := transport.WrapError(err, "failed to update resource")
		return b.updateFailureResult(request.NativeID,
			transport.ToResourceErrorCode(transportErr.Code),
			transportErr.Message), nil
	}

	// Handle synchronous operations
	if b.OperationConfig.Synchronous {
		// Return the updated resource's properties, exactly as
		// handleSynchronousCreate does. Without this the caller keeps the
		// pre-update state: there is no operation to poll and so no later
		// read-back, and every changed field reads as missing until the next
		// sync.
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:          resource.OperationUpdate,
				OperationStatus:    resource.OperationStatusSuccess,
				NativeID:           request.NativeID,
				StatusMessage:      "Resource updated successfully",
				ResourceProperties: b.readBackAfterUpdate(ctx, request),
			},
		}, nil
	}

	// Extract operation ID for async operations
	operationID := b.OperationConfig.OperationIDExtractor(response.Body)
	requestID := b.OperationConfig.OperationURLBuilder(pathCtx, operationID)

	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        request.NativeID,
			RequestID:       requestID,
			StatusMessage:   fmt.Sprintf("%s update in progress", b.ResourceConfig.ResourceType),
		},
	}, nil
}

// updateWithOptimisticLocking handles updates that require optimistic locking
func (b *BaseResource) updateWithOptimisticLocking(
	ctx context.Context,
	client *transport.Client,
	request *resource.UpdateRequest,
	props map[string]interface{},
	pathCtx PathContext,
) (*resource.UpdateResult, error) {
	// First, read the current resource to get the locking field
	urlBuilder := NewURLBuilder(b.APIConfig, pathCtx)
	url := urlBuilder.ResourceURL(pathCtx.ResourceName)

	getResponse, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    url,
	})
	if err != nil {
		wrappedErr := transport.WrapError(err, "failed to read resource for locking")
		return b.updateFailureResult(request.NativeID,
			transport.ToResourceErrorCode(wrappedErr.Code),
			wrappedErr.Message), nil
	}

	// Extract locking field. Most GCP etags are strings ("fingerprint",
	// "labelFingerprint", "etag"), but some are numbers - Dataproc's
	// workflowTemplates.version is an int - so keep the raw value for the body
	// and derive a string only for the query-parameter case. A string assertion
	// alone silently yielded "" and the API rejected the update.
	lockingField := b.ResourceConfig.OptimisticLocking.FieldName
	lockingRaw := getResponse.Body[lockingField]
	lockingValue := lockingValueString(lockingRaw)

	// Transform request properties
	body := props
	if b.RequestTransformer != nil {
		transformCtx := b.buildTransformContext(pathCtx, resource.OperationUpdate)
		body, err = b.RequestTransformer.Transform(props, transformCtx)
		if err != nil {
			return b.updateFailureResult(request.NativeID,
				resource.OperationErrorCodeInvalidRequest,
				fmt.Sprintf("failed to transform request: %v", err)), nil
		}
	}

	// Add locking field to body or URL
	if b.ResourceConfig.OptimisticLocking.LocationInURL {
		// Add as query parameter
		url, _ = transport.AddQueryParam(url, lockingField, lockingValue)
	} else if lockingRaw != nil {
		// Add to request body, keeping the API's own type: a numeric version
		// must not become the string "2".
		body[lockingField] = lockingRaw
	}

	// Apply request wrapper if configured
	if b.ResourceConfig.RequestWrapper != "" {
		body = map[string]interface{}{
			b.ResourceConfig.RequestWrapper: body,
		}
	}

	// Use configured HTTP method (PATCH or PUT)
	httpMethod := b.ResourceConfig.GetUpdateMethod()

	// Add any configured update query parameters
	if len(b.ResourceConfig.UpdateQueryParams) > 0 {
		url, _ = transport.AddQueryParams(url, b.ResourceConfig.UpdateQueryParams)
	}

	response, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: httpMethod,
		URL:    url,
		Body:   body,
	})
	if err != nil {
		transportErr := transport.WrapError(err, "failed to update resource")
		return b.updateFailureResult(request.NativeID,
			transport.ToResourceErrorCode(transportErr.Code),
			transportErr.Message), nil
	}

	// Handle synchronous operations
	if b.OperationConfig.Synchronous {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusSuccess,
				NativeID:        request.NativeID,
				StatusMessage:   "Resource updated successfully",
				// As in performUpdate: without the read-back the update reports no
				// properties at all, so anything the update changed is missing from
				// state until some later sync happens to pick it up.
				ResourceProperties: b.readBackAfterUpdate(ctx, request),
			},
		}, nil
	}

	// Extract operation ID for async operations
	operationID := b.OperationConfig.OperationIDExtractor(response.Body)
	requestID := b.OperationConfig.OperationURLBuilder(pathCtx, operationID)

	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusInProgress,
			NativeID:        request.NativeID,
			RequestID:       requestID,
			StatusMessage:   fmt.Sprintf("%s update in progress", b.ResourceConfig.ResourceType),
		},
	}, nil
}

// parseListResponse parses list response - can be overridden by API-specific implementations
func (b *BaseResource) parseListResponse(
	responseBody map[string]interface{},
	pathCtx PathContext,
) (*resource.ListResult, error) {
	var nativeIDs []string

	// Try "items" first (common pattern)
	if items, ok := responseBody["items"].([]interface{}); ok {
		// Simple array of items
		for _, item := range items {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if nativeID := b.extractNativeIDFromItem(itemMap, pathCtx); nativeID != "" {
					nativeIDs = append(nativeIDs, nativeID)
				}
			}
		}
	} else if items, ok := responseBody["items"].(map[string]interface{}); ok {
		// Aggregated list format: items is a map of zones/regions to resource lists
		// Example: {"zones/us-central1-a": {"instances": [...]}, "zones/us-central1-b": {"instances": [...]}}
		for _, zoneData := range items {
			if zoneMap, ok := zoneData.(map[string]interface{}); ok {
				// Look for the resource type key within each zone/region
				if resourceList, ok := zoneMap[b.ResourceConfig.ResourceType].([]interface{}); ok {
					for _, item := range resourceList {
						if itemMap, ok := item.(map[string]interface{}); ok {
							if nativeID := b.extractNativeIDFromItem(itemMap, pathCtx); nativeID != "" {
								nativeIDs = append(nativeIDs, nativeID)
							}
						}
					}
				}
			}
		}
	} else if items, ok := responseBody[b.ResourceConfig.ResourceType].([]interface{}); ok {
		// Try resource type key (Container API pattern)
		for _, item := range items {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if nativeID := b.extractNativeIDFromItem(itemMap, pathCtx); nativeID != "" {
					nativeIDs = append(nativeIDs, nativeID)
				}
			}
		}
	} else if key := b.ResourceConfig.ListItemsKey; key != "" {
		// Try the configured items key (e.g. IAM serviceAccounts.list -> "accounts")
		if items, ok := responseBody[key].([]interface{}); ok {
			for _, item := range items {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if nativeID := b.extractNativeIDFromItem(itemMap, pathCtx); nativeID != "" {
						nativeIDs = append(nativeIDs, nativeID)
					}
				}
			}
		}
	}

	// Extract next page token
	var nextToken *string
	if token, ok := responseBody["nextPageToken"].(string); ok && token != "" {
		nextToken = &token
	}

	return &resource.ListResult{
		NativeIDs:     nativeIDs,
		NextPageToken: nextToken,
	}, nil
}

// extractNativeIDFromItem extracts the native ID from a list item
func (b *BaseResource) extractNativeIDFromItem(itemMap map[string]interface{}, pathCtx PathContext) string {
	// Ask the API's own extractor first. Requiring a "name" before doing so
	// made every resource identified by something else undiscoverable: a Cloud
	// Storage ACL entry is identified by "entity" and has no name at all, so
	// each listed item yielded nothing and the list came back empty - with no
	// error, which is indistinguishable from "none exist".
	var nativeID string
	if b.OperationConfig.NativeIDExtractor != nil {
		// Handles selfLink/targetLink and API-specific identity fields.
		nativeID = b.OperationConfig.NativeIDExtractor(itemMap, pathCtx)
	}
	if nativeID != "" {
		return nativeID
	}

	// Fall back to the name-shaped path, which needs a name to build one.
	name, _ := itemMap["name"].(string)
	if name == "" {
		return ""
	}
	return BuildNativeID(b.NativeIDConfig, name, pathCtx)
}

// Failure result helpers

func (b *BaseResource) createFailureResult(errorCode resource.OperationErrorCode, message string) *resource.CreateResult {
	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       errorCode,
			StatusMessage:   message,
		},
	}
}

func (b *BaseResource) updateFailureResult(nativeID string, errorCode resource.OperationErrorCode, message string) *resource.UpdateResult {
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       errorCode,
			StatusMessage:   message,
			NativeID:        nativeID,
		},
	}
}

func (b *BaseResource) deleteFailureResult(nativeID string, errorCode resource.OperationErrorCode, message string) *resource.DeleteResult {
	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusFailure,
			ErrorCode:       errorCode,
			StatusMessage:   message,
			NativeID:        nativeID,
		},
	}
}

// UnwrapValues replaces formae's wrapped-value objects with the value inside.
//
// A forma can wrap a property to control how it is shown or stored - notably
// `formae.value(secret).opaque`, which keeps a secret out of plans and state.
// Formae unwraps those before calling a plugin, so the normal apply path never
// sees them. The conformance harness's out-of-band path calls the plugin
// directly with the evaluated forma, wrappers intact, and a wrapper sent as a
// field value reaches the API as an object where it expects a string - GCP
// reports it as empty ("A shared secret must be..."). Unwrapping here makes the
// plugin tolerant of both shapes.
func UnwrapValues(props map[string]interface{}) map[string]interface{} {
	unwrapped, _ := unwrapValue(props).(map[string]interface{})
	if unwrapped == nil {
		return props
	}
	return unwrapped
}

func unwrapValue(v interface{}) interface{} {
	switch value := v.(type) {
	case map[string]interface{}:
		// A wrapper carries "$value" alongside only "$"-prefixed siblings; a
		// resolvable ("$res") is left alone, since formae resolves those and a
		// half-resolved reference must not be mistaken for a literal.
		if inner, ok := value["$value"]; ok {
			if _, isRef := value["$res"]; !isRef {
				return unwrapValue(inner)
			}
		}
		out := make(map[string]interface{}, len(value))
		for k, item := range value {
			out[k] = unwrapValue(item)
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(value))
		for _, item := range value {
			out = append(out, unwrapValue(item))
		}
		return out
	default:
		return v
	}
}

// lockingValueString renders an optimistic-locking value for a URL query
// parameter. Strings pass through; numeric etags are formatted without a decimal
// point, since JSON decodes every number to float64.
func lockingValueString(raw interface{}) string {
	switch v := raw.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

// readBackAfterUpdate returns the resource as Read would report it, so stored
// state after a synchronous update has exactly the shape it has after a create
// or a sync.
//
// The update *response* is not a safe substitute: some APIs echo fields their
// GET omits - a Storage bucket's PATCH returns defaultObjectAcl, which the read
// path does not - and storing those makes conformance report a property that is
// "not expected and not a provider default". A failed read-back is not a failed
// update; the operation already succeeded, so it yields nil and the next sync
// fills state in.
func (b *BaseResource) readBackAfterUpdate(ctx context.Context, request *resource.UpdateRequest) json.RawMessage {
	if request.NativeID == "" {
		return nil
	}
	readResult, err := b.Read(ctx, &resource.ReadRequest{
		NativeID:     request.NativeID,
		ResourceType: request.ResourceType,
		TargetConfig: request.TargetConfig,
	})
	if err != nil || readResult == nil || readResult.ErrorCode != "" || readResult.Properties == "" {
		return nil
	}
	return json.RawMessage(readResult.Properties)
}
