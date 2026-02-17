// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package cloudrun

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
)

// jobBodyBuilder builds the request body for creating a Cloud Run job
func jobBodyBuilder(props map[string]interface{}) (map[string]interface{}, error) {
	body := make(map[string]interface{})

	// Labels
	if labels := getStringMap(props, "labels"); labels != nil {
		body["labels"] = labels
	}

	// Annotations
	if annotations := getStringMap(props, "annotations"); annotations != nil {
		body["annotations"] = annotations
	}

	// Description
	if desc := utils.GetString(props, "description"); desc != "" {
		body["description"] = desc
	}

	// Template (execution template)
	if templateProps := utils.GetObject(props, "template"); templateProps != nil {
		body["template"] = buildExecutionTemplate(templateProps)
	}

	// Launch stage
	if launchStage := utils.GetString(props, "launchStage"); launchStage != "" {
		body["launchStage"] = launchStage
	}

	// Binary authorization
	if binaryAuth := utils.GetObject(props, "binaryAuthorization"); binaryAuth != nil {
		body["binaryAuthorization"] = binaryAuth
	}

	// Client
	if client := utils.GetString(props, "client"); client != "" {
		body["client"] = client
	}

	// Client version
	if clientVersion := utils.GetString(props, "clientVersion"); clientVersion != "" {
		body["clientVersion"] = clientVersion
	}

	return body, nil
}

// buildExecutionTemplate builds the execution template for a Cloud Run job
func buildExecutionTemplate(templateProps map[string]interface{}) map[string]interface{} {
	execTemplate := make(map[string]interface{})

	// Task count
	if taskCount := utils.GetInt32(templateProps, "taskCount"); taskCount > 0 {
		execTemplate["taskCount"] = taskCount
	}

	// Parallelism
	if parallelism := utils.GetInt32(templateProps, "parallelism"); parallelism > 0 {
		execTemplate["parallelism"] = parallelism
	}

	// Labels
	if labels := getStringMap(templateProps, "labels"); labels != nil {
		execTemplate["labels"] = labels
	}

	// Annotations
	if annotations := getStringMap(templateProps, "annotations"); annotations != nil {
		execTemplate["annotations"] = annotations
	}

	// Task template
	if taskTemplateProps := utils.GetObject(templateProps, "template"); taskTemplateProps != nil {
		execTemplate["template"] = buildTaskTemplate(taskTemplateProps)
	}

	return execTemplate
}

// buildTaskTemplate builds the task template for a Cloud Run job execution
func buildTaskTemplate(taskProps map[string]interface{}) map[string]interface{} {
	taskTemplate := make(map[string]interface{})

	// Containers
	if containers := getContainersArray(taskProps); containers != nil {
		taskTemplate["containers"] = containers
	}

	// Volumes
	if volumes := getVolumesArray(taskProps); volumes != nil {
		taskTemplate["volumes"] = volumes
	}

	// Service account
	if sa := utils.GetString(taskProps, "serviceAccount"); sa != "" {
		taskTemplate["serviceAccount"] = sa
	}

	// Execution environment
	if execEnv := utils.GetString(taskProps, "executionEnvironment"); execEnv != "" {
		taskTemplate["executionEnvironment"] = execEnv
	}

	// Max retries
	if maxRetries := utils.GetInt32(taskProps, "maxRetries"); maxRetries > 0 {
		taskTemplate["maxRetries"] = maxRetries
	}

	// Timeout
	if timeout := utils.GetString(taskProps, "timeout"); timeout != "" {
		taskTemplate["timeout"] = timeout
	}

	// VPC access
	if vpcAccess := utils.GetObject(taskProps, "vpcAccess"); vpcAccess != nil {
		taskTemplate["vpcAccess"] = buildVpcAccess(vpcAccess)
	}

	return taskTemplate
}

// jobResponseTransformer transforms the API response into a normalized format
func jobResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	props := make(map[string]interface{})

	// Basic fields - normalize name to short form
	if name, ok := apiResponse["name"].(string); ok {
		props["name"] = base.ExtractLastSegment(name)
	}
	if uid, ok := apiResponse["uid"].(string); ok {
		props["uid"] = uid
	}

	// Labels
	if labels, ok := apiResponse["labels"].(map[string]interface{}); ok {
		props["labels"] = labels
	}

	// Annotations
	if annotations, ok := apiResponse["annotations"].(map[string]interface{}); ok {
		props["annotations"] = annotations
	}

	// Template
	if template, ok := apiResponse["template"].(map[string]interface{}); ok {
		props["template"] = template
	}

	// Latest created execution
	if latestExecution, ok := apiResponse["latestCreatedExecution"].(map[string]interface{}); ok {
		if execName, ok := latestExecution["name"].(string); ok {
			props["latestCreatedExecution"] = execName
		}
	}

	// Add location/region from context
	if ctx.Location != "" {
		props["location"] = ctx.Location
	}
	props["project"] = ctx.Project
	if ctx.Region != "" {
		props["region"] = ctx.Region
	} else if ctx.Location != "" {
		props["region"] = ctx.Location
	}

	return props
}
