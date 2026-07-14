// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package sql

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
)

// SQLAPI defines the API configuration for GCP Cloud SQL API
var SQLAPI = base.APIConfig{
	BaseURL:     "https://sqladmin.googleapis.com/v1",
	APIVersion:  "v1",
	PathBuilder: sqlPathBuilder,
}

// SQLOperations defines how operations work in the SQL API
var SQLOperations = base.OperationConfig{
	Synchronous: false, // SQL operations are asynchronous

	// Extract operation name from response
	OperationIDExtractor: func(response map[string]interface{}) string {
		return utils.GetString(response, "name")
	},

	// Build operation URL: projects/{project}/operations/{operation}
	OperationURLBuilder: func(ctx base.PathContext, operationID string) string {
		return fmt.Sprintf("projects/%s/operations/%s", ctx.Project, operationID)
	},

	// Extract native ID from response or operation
	NativeIDExtractor: extractSQLNativeID,

	// Check if operation is complete
	OperationStatusChecker: func(response map[string]interface{}) (bool, error) {
		status := utils.GetString(response, "status")
		isDone := status == "DONE"

		// Check for errors in the operation
		if isDone {
			if errorObj, ok := response["error"].(map[string]interface{}); ok {
				if errors, ok := errorObj["errors"].([]interface{}); ok && len(errors) > 0 {
					if firstErr, ok := errors[0].(map[string]interface{}); ok {
						errorMsg := utils.GetString(firstErr, "message")
						return true, fmt.Errorf("operation failed: %s", errorMsg)
					}
				}
			}
		}

		return isDone, nil
	},

	// A database delete right after its client disconnects can fail with
	// "database ... is being accessed by other users" — Cloud SQL reaps lingering
	// sessions on a lag. Treat it as retryable so formae core re-runs the delete
	// until the sessions clear, instead of failing the teardown.
	RetryableError: func(err error) bool {
		return err != nil && strings.Contains(err.Error(), "is being accessed by other users")
	},
}

// SQLNativeID defines native ID format for SQL resources
var SQLNativeID = base.NativeIDConfig{
	Format:       base.FullPathFormat,
	PathTemplate: "projects/{project}/{resourceType}/{name}",
	Parser:       parseSQLNativeID,
}

// sqlPathBuilder builds SQL API paths.
// Top-level:  /projects/{project}/{resourceType}/{name}          (e.g. instances)
// Nested:     /projects/{project}/{parentType}/{parent}/{resourceType}/{name}
//             (e.g. databases under an instance)
func sqlPathBuilder(ctx base.PathContext) string {
	prefix := fmt.Sprintf("/projects/%s", ctx.Project)
	if ctx.ParentType != "" && ctx.ParentResource != "" {
		prefix = fmt.Sprintf("%s/%s/%s", prefix, ctx.ParentType, ctx.ParentResource)
	}
	if ctx.ResourceName != "" {
		return fmt.Sprintf("%s/%s/%s", prefix, ctx.ResourceType, ctx.ResourceName)
	}
	return fmt.Sprintf("%s/%s", prefix, ctx.ResourceType)
}

// extractSQLNativeID extracts the native ID from SQL API response
func extractSQLNativeID(response map[string]interface{}, ctx base.PathContext) string {
	// For operations, check targetLink first
	if targetLink, ok := response["targetLink"].(string); ok {
		return utils.SelfLinkToNativeID(targetLink)
	}

	// Extract name from response
	name := utils.GetString(response, "name")
	if name == "" {
		return ""
	}

	// Nested resource (e.g. a database under an instance):
	// projects/{project}/{parentType}/{parent}/{resourceType}/{name}
	if ctx.ParentType != "" && ctx.ParentResource != "" {
		return fmt.Sprintf("projects/%s/%s/%s/%s/%s", ctx.Project, ctx.ParentType, ctx.ParentResource, ctx.ResourceType, name)
	}

	// Top-level resource: projects/{project}/{resourceType}/{name}
	return fmt.Sprintf("projects/%s/%s/%s", ctx.Project, ctx.ResourceType, name)
}

// parseSQLNativeID parses a SQL native ID into PathContext. Handles both the
// top-level form (projects/{project}/{resourceType}/{name}) and the nested form
// (projects/{project}/{parentType}/{parent}/{resourceType}/{name}).
func parseSQLNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if parts[0] != "projects" {
		return base.PathContext{}, fmt.Errorf("invalid SQL native ID format: %s", nativeID)
	}

	switch len(parts) {
	case 4: // projects/{project}/{resourceType}/{name}
		return base.PathContext{
			Project:      parts[1],
			ResourceType: parts[2],
			ResourceName: parts[3],
		}, nil
	case 6: // projects/{project}/{parentType}/{parent}/{resourceType}/{name}
		return base.PathContext{
			Project:        parts[1],
			ParentType:     parts[2],
			ParentResource: parts[3],
			ResourceType:   parts[4],
			ResourceName:   parts[5],
		}, nil
	default:
		return base.PathContext{}, fmt.Errorf("invalid SQL native ID format: %s (expected 4 or 6 path segments)", nativeID)
	}
}
