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

	// Remove location from props (passed as URL parameter)
	if location := utils.GetString(props, "location"); location != "" {
		delete(props, "location")
	}

	// Remove name from props (passed as job_id URL parameter)
	if name := utils.GetString(props, "name"); name != "" {
		delete(props, "name")
	}

	return props, nil
}

// buildExecutionTemplate builds the execution template for a Cloud Run job
func buildExecutionTemplate(templateProps map[string]interface{}) map[string]interface{} {
	template := make(map[string]interface{})

	// Task count
	if taskCount := utils.GetInt32(templateProps, "taskCount"); taskCount > 0 {
		template["taskCount"] = taskCount
	}

	// Parallelism
	if parallelism := utils.GetInt32(templateProps, "parallelism"); parallelism > 0 {
		template["parallelism"] = parallelism
	}

	// Task template
	if taskTemplateProps := utils.GetObject(templateProps, "template"); taskTemplateProps != nil {
		template["template"] = buildTaskTemplate(taskTemplateProps)
	}

	return template
}

// buildTaskTemplate builds the task template for a Cloud Run job
func buildTaskTemplate(taskProps map[string]interface{}) map[string]interface{} {
	template := make(map[string]interface{})

	// Containers
	if containers := getJobContainersArray(taskProps); containers != nil {
		template["containers"] = containers
	}

	// Volumes
	if volumes := getJobVolumesArray(taskProps); volumes != nil {
		template["volumes"] = volumes
	}

	// Service account
	if serviceAccount := utils.GetString(taskProps, "serviceAccount"); serviceAccount != "" {
		template["serviceAccount"] = serviceAccount
	}

	// Execution environment
	if execEnv := utils.GetString(taskProps, "executionEnvironment"); execEnv != "" {
		template["executionEnvironment"] = execEnv
	}

	// Max retries
	if maxRetries := utils.GetInt32(taskProps, "maxRetries"); maxRetries > 0 {
		template["maxRetries"] = maxRetries
	}

	// Timeout
	if timeout := utils.GetString(taskProps, "timeout"); timeout != "" {
		template["timeout"] = timeout
	}

	// VPC Access
	if vpcAccess := utils.GetObject(taskProps, "vpcAccess"); vpcAccess != nil {
		template["vpcAccess"] = buildJobVpcAccess(vpcAccess)
	}

	return template
}

// getJobContainersArray builds the containers array for jobs
func getJobContainersArray(props map[string]interface{}) []map[string]interface{} {
	if val, ok := props["containers"]; ok {
		if arr, ok := val.([]interface{}); ok {
			result := make([]map[string]interface{}, 0, len(arr))
			for _, item := range arr {
				if obj, ok := item.(map[string]interface{}); ok {
					container := make(map[string]interface{})

					if name := utils.GetString(obj, "name"); name != "" {
						container["name"] = name
					}
					if image := utils.GetString(obj, "image"); image != "" {
						container["image"] = image
					}

					// Command
					if command := getStringArray(obj, "command"); command != nil {
						container["command"] = command
					}

					// Args
					if args := getStringArray(obj, "args"); args != nil {
						container["args"] = args
					}

					// Working directory
					if workingDir := utils.GetString(obj, "workingDir"); workingDir != "" {
						container["workingDir"] = workingDir
					}

					// Environment variables
					if env := getEnvArray(obj); env != nil {
						container["env"] = env
					}

					// Resources
					if resources := utils.GetObject(obj, "resources"); resources != nil {
						resourcesMap := make(map[string]interface{})
						if limits := getStringMap(resources, "limits"); limits != nil {
							resourcesMap["limits"] = limits
						}
						if len(resourcesMap) > 0 {
							container["resources"] = resourcesMap
						}
					}

					// Volume mounts
					if volumeMounts := getVolumeMountsArray(obj); volumeMounts != nil {
						container["volumeMounts"] = volumeMounts
					}

					result = append(result, container)
				}
			}
			return result
		}
	}
	return nil
}

// getJobVolumesArray builds the volumes array for jobs
func getJobVolumesArray(props map[string]interface{}) []map[string]interface{} {
	if val, ok := props["volumes"]; ok {
		if arr, ok := val.([]interface{}); ok {
			result := make([]map[string]interface{}, 0, len(arr))
			for _, item := range arr {
				if obj, ok := item.(map[string]interface{}); ok {
					volume := make(map[string]interface{})
					if name := utils.GetString(obj, "name"); name != "" {
						volume["name"] = name
					}

					// Secret volume
					if secret := utils.GetObject(obj, "secret"); secret != nil {
						secretMap := make(map[string]interface{})
						if secretName := utils.GetString(secret, "secret"); secretName != "" {
							secretMap["secret"] = secretName
						}
						if defaultMode := utils.GetInt32(secret, "defaultMode"); defaultMode > 0 {
							secretMap["defaultMode"] = defaultMode
						}
						volume["secret"] = secretMap
					}

					// Cloud SQL instance
					if cloudSql := utils.GetObject(obj, "cloudSqlInstance"); cloudSql != nil {
						if instances := getStringArray(cloudSql, "instances"); instances != nil {
							volume["cloudSqlInstance"] = map[string]interface{}{
								"instances": instances,
							}
						}
					}

					// Empty dir
					if emptyDir := utils.GetObject(obj, "emptyDir"); emptyDir != nil {
						emptyDirMap := make(map[string]interface{})
						if medium := utils.GetString(emptyDir, "medium"); medium != "" {
							emptyDirMap["medium"] = medium
						}
						if sizeLimit := utils.GetString(emptyDir, "sizeLimit"); sizeLimit != "" {
							emptyDirMap["sizeLimit"] = sizeLimit
						}
						volume["emptyDir"] = emptyDirMap
					}

					result = append(result, volume)
				}
			}
			return result
		}
	}
	return nil
}

// buildJobVpcAccess builds the VPC access configuration for jobs
func buildJobVpcAccess(vpcProps map[string]interface{}) map[string]interface{} {
	vpcAccess := make(map[string]interface{})

	// Connector
	if connector := utils.GetString(vpcProps, "connector"); connector != "" {
		vpcAccess["connector"] = connector
	}

	// Egress
	if egress := utils.GetString(vpcProps, "egress"); egress != "" {
		vpcAccess["egress"] = egress
	}

	// Network interfaces
	if networkInterfaces := getJobNetworkInterfacesArray(vpcProps); networkInterfaces != nil {
		vpcAccess["networkInterfaces"] = networkInterfaces
	}

	return vpcAccess
}

// getJobNetworkInterfacesArray builds the network interfaces array for jobs
func getJobNetworkInterfacesArray(props map[string]interface{}) []map[string]interface{} {
	if val, ok := props["networkInterfaces"]; ok {
		if arr, ok := val.([]interface{}); ok {
			result := make([]map[string]interface{}, 0, len(arr))
			for _, item := range arr {
				if obj, ok := item.(map[string]interface{}); ok {
					ni := make(map[string]interface{})
					if network := utils.GetString(obj, "network"); network != "" {
						ni["network"] = network
					}
					if subnetwork := utils.GetString(obj, "subnetwork"); subnetwork != "" {
						ni["subnetwork"] = subnetwork
					}
					if tags := getStringArray(obj, "tags"); tags != nil {
						ni["tags"] = tags
					}
					result = append(result, ni)
				}
			}
			return result
		}
	}
	return nil
}

// jobResponseTransformer transforms the API response into a normalized format
func jobResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	props := make(map[string]interface{})

	// Basic fields
	if name, ok := apiResponse["name"].(string); ok {
		props["name"] = name
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

	// Add location if not present (from context)
	if _, ok := props["location"]; !ok && ctx.Location != "" {
		props["location"] = ctx.Location
	}

	return props
}
