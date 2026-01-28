// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
)

// instanceBodyBuilder builds the request body for creating a Compute Instance
func instanceBodyBuilder(props map[string]interface{}, ctx base.TransformContext) (map[string]interface{}, error) {
	body := make(map[string]interface{})

	// Name (required)
	if name := utils.GetString(props, "name"); name != "" {
		body["name"] = name
	}

	// Machine type (required) - Must be in format zones/{zone}/machineTypes/{type}
	if machineType := utils.GetString(props, "machineType"); machineType != "" {
		// If not already in full path format, convert it
		if !strings.HasPrefix(machineType, "zones/") {
			zone := utils.GetString(props, "zone")
			if zone == "" {
				zone = ctx.Region + "-a" // Default to region-a
			}
			machineType = fmt.Sprintf("zones/%s/machineTypes/%s", zone, machineType)
		}
		body["machineType"] = machineType
	}

	// Description
	if description := utils.GetString(props, "description"); description != "" {
		body["description"] = description
	}

	// Network interfaces
	if networkInterfaces := utils.GetArray(props, "networkInterfaces"); len(networkInterfaces) > 0 {
		body["networkInterfaces"] = buildNetworkInterfaces(networkInterfaces)
	}

	// Disks
	if disks := utils.GetArray(props, "disks"); len(disks) > 0 {
		body["disks"] = buildDisks(disks, ctx.Project, ctx.Zone)
	}

	// Service accounts
	if serviceAccounts := utils.GetArray(props, "serviceAccounts"); len(serviceAccounts) > 0 {
		body["serviceAccounts"] = buildServiceAccounts(serviceAccounts)
	}

	// Metadata
	if metadataMap := utils.GetObject(props, "metadata"); metadataMap != nil {
		body["metadata"] = buildMetadata(metadataMap)
	}

	// Tags
	if tagsArray := utils.GetArray(props, "tags"); len(tagsArray) > 0 {
		tags := make([]string, 0, len(tagsArray))
		for _, t := range tagsArray {
			if str, ok := t.(string); ok {
				tags = append(tags, str)
			}
		}
		body["tags"] = map[string]interface{}{
			"items": tags,
		}
	}

	// Labels
	if labelsMap := utils.GetObject(props, "labels"); labelsMap != nil {
		labels := make(map[string]string)
		for k, v := range labelsMap {
			if str, ok := v.(string); ok {
				labels[k] = str
			}
		}
		body["labels"] = labels
	}

	// Scheduling
	if schedulingMap := utils.GetObject(props, "scheduling"); schedulingMap != nil {
		body["scheduling"] = buildScheduling(schedulingMap)
	}

	// Shielded instance config
	if shieldedConfigMap := utils.GetObject(props, "shieldedInstanceConfig"); shieldedConfigMap != nil {
		body["shieldedInstanceConfig"] = map[string]interface{}{
			"enableSecureBoot":          utils.GetBool(shieldedConfigMap, "enableSecureBoot"),
			"enableVtpm":                utils.GetBool(shieldedConfigMap, "enableVtpm"),
			"enableIntegrityMonitoring": utils.GetBool(shieldedConfigMap, "enableIntegrityMonitoring"),
		}
	}

	// Fingerprint - required for PUT updates (optimistic locking for entire resource)
	// This is different from labelFingerprint which is only for labels
	if fingerprint := utils.GetString(props, "fingerprint"); fingerprint != "" {
		body["fingerprint"] = fingerprint
	}

	return body, nil
}

// instanceResponseTransformer transforms the API response into a normalized format
func instanceResponseTransformer(apiResponse map[string]interface{}, ctx base.TransformContext) map[string]interface{} {
	// Extract project from selfLink and add to response
	// selfLink format: https://www.googleapis.com/compute/v1/projects/{project}/zones/{zone}/instances/{instance}
	if selfLink, ok := apiResponse["selfLink"].(string); ok && selfLink != "" {
		if project := extractProjectFromSelfLink(selfLink); project != "" {
			apiResponse["project"] = project
		}
	}

	return apiResponse
}

// extractProjectFromSelfLink extracts the project from a Compute API selfLink
// Example: https://www.googleapis.com/compute/v1/projects/my-project/zones/us-central1-a/instances/my-instance
// Returns: my-project
func extractProjectFromSelfLink(selfLink string) string {
	const projectsPrefix = "/projects/"
	idx := strings.Index(selfLink, projectsPrefix)
	if idx == -1 {
		return ""
	}

	// Start after "/projects/"
	start := idx + len(projectsPrefix)
	remaining := selfLink[start:]

	// Find the next "/"
	endIdx := strings.Index(remaining, "/")
	if endIdx == -1 {
		return remaining
	}

	return remaining[:endIdx]
}

// Helper functions for request body building

func buildNetworkInterfaces(interfaces []interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(interfaces))
	for _, iface := range interfaces {
		ifaceMap, ok := iface.(map[string]interface{})
		if !ok {
			continue
		}

		ni := make(map[string]interface{})

		if network := utils.GetString(ifaceMap, "network"); network != "" {
			ni["network"] = network
		}
		if subnetwork := utils.GetString(ifaceMap, "subnetwork"); subnetwork != "" {
			ni["subnetwork"] = subnetwork
		}

		// Access configs
		if accessConfigs := utils.GetArray(ifaceMap, "accessConfigs"); len(accessConfigs) > 0 {
			ni["accessConfigs"] = buildAccessConfigs(accessConfigs)
		}

		result = append(result, ni)
	}
	return result
}

func buildAccessConfigs(configs []interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(configs))
	for _, cfg := range configs {
		cfgMap, ok := cfg.(map[string]interface{})
		if !ok {
			continue
		}

		ac := make(map[string]interface{})
		if name := utils.GetString(cfgMap, "name"); name != "" {
			ac["name"] = name
		}
		if acType := utils.GetString(cfgMap, "type"); acType != "" {
			ac["type"] = acType
		}
		if natIP := utils.GetString(cfgMap, "natIP"); natIP != "" {
			ac["natIP"] = natIP
		}

		result = append(result, ac)
	}
	return result
}

func buildDisks(disks []interface{}, project, zone string) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(disks))
	for _, disk := range disks {
		diskMap, ok := disk.(map[string]interface{})
		if !ok {
			continue
		}

		ad := make(map[string]interface{})
		ad["boot"] = utils.GetBool(diskMap, "boot")
		ad["autoDelete"] = utils.GetBool(diskMap, "autoDelete")

		if source := utils.GetString(diskMap, "source"); source != "" {
			ad["source"] = source
		}

		if initParams := utils.GetObject(diskMap, "initializeParams"); initParams != nil {
			ip := make(map[string]interface{})
			if diskSizeGb := utils.GetInt64(initParams, "diskSizeGb"); diskSizeGb > 0 {
				ip["diskSizeGb"] = diskSizeGb
			}
			if diskType := utils.GetString(initParams, "diskType"); diskType != "" {
				// Convert short form to full path if needed
				if !strings.HasPrefix(diskType, "zones/") {
					diskType = fmt.Sprintf("zones/%s/diskTypes/%s", zone, diskType)
				}
				ip["diskType"] = diskType
			}
			if sourceImage := utils.GetString(initParams, "sourceImage"); sourceImage != "" {
				ip["sourceImage"] = sourceImage
			}
			ad["initializeParams"] = ip
		}

		result = append(result, ad)
	}
	return result
}

func buildServiceAccounts(accounts []interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(accounts))
	for _, acct := range accounts {
		acctMap, ok := acct.(map[string]interface{})
		if !ok {
			continue
		}

		sa := make(map[string]interface{})
		if email := utils.GetString(acctMap, "email"); email != "" {
			sa["email"] = email
		}

		if scopes := utils.GetArray(acctMap, "scopes"); len(scopes) > 0 {
			scopeStrs := make([]string, 0, len(scopes))
			for _, s := range scopes {
				if str, ok := s.(string); ok {
					scopeStrs = append(scopeStrs, str)
				}
			}
			sa["scopes"] = scopeStrs
		}

		result = append(result, sa)
	}
	return result
}

func buildMetadata(metadataMap map[string]interface{}) map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(metadataMap))
	for key, value := range metadataMap {
		if str, ok := value.(string); ok {
			items = append(items, map[string]interface{}{
				"key":   key,
				"value": str,
			})
		}
	}
	return map[string]interface{}{
		"items": items,
	}
}

func buildScheduling(schedulingMap map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"preemptible":       utils.GetBool(schedulingMap, "preemptible"),
		"onHostMaintenance": utils.GetString(schedulingMap, "onHostMaintenance"),
		"automaticRestart":  utils.GetBool(schedulingMap, "automaticRestart"),
	}
}
