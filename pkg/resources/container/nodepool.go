package container

import (
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
)

func nodePoolBodyBuilder(props map[string]interface{}) (map[string]interface{}, error) {
	clusterName := utils.GetString(props, "cluster")
	if clusterName == "" {
		return nil, fmt.Errorf("cluster field is required for node pool")
	}

	location := utils.GetString(props, "location")
	if location == "" {
		return nil, fmt.Errorf("location field is required for node pool")
	}

	// Parse cluster name to extract project (if it's a full path)
	// Format might be: projects/{project}/locations/{location}/clusters/{cluster}
	// or just the cluster name
	var projectId string
	if clusterPathComponents, err := NewClusterPathComponents(clusterName); err == nil {
		// It's a full path, extract project
		projectId = clusterPathComponents.Project
	}
	// If not a full path or parsing failed, projectId will remain empty
	// and will be handled by the parent path in the URL

	// Create a copy of props without metadata fields (cluster, location)
	// These fields are routing metadata, not part of the NodePool resource schema
	nodePoolProps := make(map[string]interface{})
	for k, v := range props {
		if k != "cluster" && k != "location" {
			nodePoolProps[k] = v
		}
	}

	// Build the request body with deprecated fields + wrapped nodePool
	body := map[string]interface{}{
		"nodePool": nodePoolProps,
	}

	if projectId != "" {
		body["projectId"] = projectId
	}
	return body, nil
}
