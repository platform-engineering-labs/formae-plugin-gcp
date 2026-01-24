// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package cloudrun

import (
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/cfres/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
)

// serviceBodyBuilder builds the request body for creating a Cloud Run service
func serviceBodyBuilder(props map[string]interface{}) (map[string]interface{}, error) {
	body := make(map[string]interface{})

	// Labels
	if labels := getStringMap(props, "labels"); labels != nil {
		body["labels"] = labels
	}

	// Annotations
	if annotations := getStringMap(props, "annotations"); annotations != nil {
		body["annotations"] = annotations
	}

	// Template (revision template)
	if templateProps := utils.GetObject(props, "template"); templateProps != nil {
		body["template"] = buildRevisionTemplate(templateProps)
	}

	// Traffic configuration
	if traffic := getTrafficArray(props); traffic != nil {
		body["traffic"] = traffic
	}

	return body, nil
}

// buildRevisionTemplate builds the revision template for a Cloud Run service
func buildRevisionTemplate(templateProps map[string]interface{}) map[string]interface{} {
	template := make(map[string]interface{})

	// Scaling
	if scaling := utils.GetObject(templateProps, "scaling"); scaling != nil {
		scalingMap := make(map[string]interface{})
		if minInstanceCount := utils.GetInt32(scaling, "minInstanceCount"); minInstanceCount > 0 {
			scalingMap["minInstanceCount"] = minInstanceCount
		}
		if maxInstanceCount := utils.GetInt32(scaling, "maxInstanceCount"); maxInstanceCount > 0 {
			scalingMap["maxInstanceCount"] = maxInstanceCount
		}
		if len(scalingMap) > 0 {
			template["scaling"] = scalingMap
		}
	}

	// Containers
	if containers := getContainersArray(templateProps); containers != nil {
		template["containers"] = containers
	}

	// Volumes
	if volumes := getVolumesArray(templateProps); volumes != nil {
		template["volumes"] = volumes
	}

	// Service account
	if serviceAccount := utils.GetString(templateProps, "serviceAccount"); serviceAccount != "" {
		template["serviceAccount"] = serviceAccount
	}

	// Timeout
	if timeout := utils.GetString(templateProps, "timeout"); timeout != "" {
		template["timeout"] = timeout
	}

	// VPC Access
	if vpcAccess := utils.GetObject(templateProps, "vpcAccess"); vpcAccess != nil {
		template["vpcAccess"] = buildVpcAccess(vpcAccess)
	}

	// Execution environment
	if execEnv := utils.GetString(templateProps, "executionEnvironment"); execEnv != "" {
		template["executionEnvironment"] = execEnv
	}

	return template
}

// getContainersArray builds the containers array
func getContainersArray(props map[string]interface{}) []map[string]interface{} {
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

					// Ports
					if ports := getPortsArray(obj); ports != nil {
						container["ports"] = ports
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
						if cpuIdle := utils.GetBool(resources, "cpuIdle"); cpuIdle {
							resourcesMap["cpuIdle"] = cpuIdle
						}
						if startupCpuBoost := utils.GetBool(resources, "startupCpuBoost"); startupCpuBoost {
							resourcesMap["startupCpuBoost"] = startupCpuBoost
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

// getPortsArray builds the ports array
func getPortsArray(props map[string]interface{}) []map[string]interface{} {
	if val, ok := props["ports"]; ok {
		if arr, ok := val.([]interface{}); ok {
			result := make([]map[string]interface{}, 0, len(arr))
			for _, item := range arr {
				if obj, ok := item.(map[string]interface{}); ok {
					port := make(map[string]interface{})
					if name := utils.GetString(obj, "name"); name != "" {
						port["name"] = name
					}
					if containerPort := utils.GetInt32(obj, "containerPort"); containerPort > 0 {
						port["containerPort"] = containerPort
					}
					result = append(result, port)
				}
			}
			return result
		}
	}
	return nil
}

// getEnvArray builds the environment variables array
func getEnvArray(props map[string]interface{}) []map[string]interface{} {
	if val, ok := props["env"]; ok {
		if arr, ok := val.([]interface{}); ok {
			result := make([]map[string]interface{}, 0, len(arr))
			for _, item := range arr {
				if obj, ok := item.(map[string]interface{}); ok {
					envVar := make(map[string]interface{})
					if name := utils.GetString(obj, "name"); name != "" {
						envVar["name"] = name
					}
					if value := utils.GetString(obj, "value"); value != "" {
						envVar["value"] = value
					}
					// Support for valueSource (secrets, etc.) could be added here
					result = append(result, envVar)
				}
			}
			return result
		}
	}
	return nil
}

// getVolumeMountsArray builds the volume mounts array
func getVolumeMountsArray(props map[string]interface{}) []map[string]interface{} {
	if val, ok := props["volumeMounts"]; ok {
		if arr, ok := val.([]interface{}); ok {
			result := make([]map[string]interface{}, 0, len(arr))
			for _, item := range arr {
				if obj, ok := item.(map[string]interface{}); ok {
					mount := make(map[string]interface{})
					if name := utils.GetString(obj, "name"); name != "" {
						mount["name"] = name
					}
					if mountPath := utils.GetString(obj, "mountPath"); mountPath != "" {
						mount["mountPath"] = mountPath
					}
					result = append(result, mount)
				}
			}
			return result
		}
	}
	return nil
}

// getVolumesArray builds the volumes array
func getVolumesArray(props map[string]interface{}) []map[string]interface{} {
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

// getTrafficArray builds the traffic array
func getTrafficArray(props map[string]interface{}) []map[string]interface{} {
	if val, ok := props["traffic"]; ok {
		if arr, ok := val.([]interface{}); ok {
			result := make([]map[string]interface{}, 0, len(arr))
			for _, item := range arr {
				if obj, ok := item.(map[string]interface{}); ok {
					traffic := make(map[string]interface{})
					if percent := utils.GetInt32(obj, "percent"); percent > 0 {
						traffic["percent"] = percent
					}
					if revision := utils.GetString(obj, "revision"); revision != "" {
						traffic["type"] = "TRAFFIC_TARGET_ALLOCATION_TYPE_REVISION"
						traffic["revision"] = revision
					} else {
						traffic["type"] = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
					}
					result = append(result, traffic)
				}
			}
			return result
		}
	}
	return nil
}

// buildVpcAccess builds the VPC access configuration for services
func buildVpcAccess(vpcProps map[string]interface{}) map[string]interface{} {
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
	if networkInterfaces := getServiceNetworkInterfacesArray(vpcProps); networkInterfaces != nil {
		vpcAccess["networkInterfaces"] = networkInterfaces
	}

	return vpcAccess
}

// getServiceNetworkInterfacesArray builds the network interfaces array for services
func getServiceNetworkInterfacesArray(props map[string]interface{}) []map[string]interface{} {
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

// serviceResponseTransformer transforms the API response into a normalized format
func serviceResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	props := make(map[string]interface{})

	// Basic fields
	if name, ok := apiResponse["name"].(string); ok {
		props["name"] = name
	}
	if uid, ok := apiResponse["uid"].(string); ok {
		props["uid"] = uid
	}
	if uri, ok := apiResponse["uri"].(string); ok {
		props["uri"] = uri
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

	// Traffic
	if traffic, ok := apiResponse["traffic"].([]interface{}); ok {
		props["traffic"] = traffic
	}

	// Add location if not present (from context)
	if _, ok := props["location"]; !ok && ctx.Location != "" {
		props["location"] = ctx.Location
	}

	props["project"] = ctx.Project
	props["region"] = ctx.Region

	return props
}
