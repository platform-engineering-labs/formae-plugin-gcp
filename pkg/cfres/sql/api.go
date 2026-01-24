// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package sql

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/base"
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
}

// SQLNativeID defines native ID format for SQL resources
var SQLNativeID = base.NativeIDConfig{
	Format:       base.FullPathFormat,
	PathTemplate: "projects/{project}/{resourceType}/{name}",
	Parser:       parseSQLNativeID,
}

// sqlPathBuilder builds SQL API paths
// Format: /projects/{project}/{resourceType}/{name}
func sqlPathBuilder(ctx base.PathContext) string {
	if ctx.ResourceName != "" {
		return fmt.Sprintf("/projects/%s/%s/%s", ctx.Project, ctx.ResourceType, ctx.ResourceName)
	}
	return fmt.Sprintf("/projects/%s/%s", ctx.Project, ctx.ResourceType)
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

	// Build full path native ID
	return fmt.Sprintf("projects/%s/%s/%s", ctx.Project, ctx.ResourceType, name)
}

// parseSQLNativeID parses a SQL native ID into PathContext
// Format: projects/{project}/{resourceType}/{name}
func parseSQLNativeID(nativeID string) (base.PathContext, error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 4 || parts[0] != "projects" {
		return base.PathContext{}, fmt.Errorf("invalid SQL native ID format: %s (expected: projects/{project}/{resourceType}/{name})", nativeID)
	}

	return base.PathContext{
		Project:      parts[1],
		ResourceType: parts[2],
		ResourceName: parts[3],
	}, nil
}
