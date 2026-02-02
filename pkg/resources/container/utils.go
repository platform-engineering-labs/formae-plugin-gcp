package container

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
)

// Resource path builders for GKE Container resources

// BuildParentPath builds the parent path for GKE resources
// Format: projects/{project}/locations/{location}
func BuildParentPath(project, location string) string {
	return fmt.Sprintf("projects/%s/locations/%s", project, location)
}

// BuildClusterPath builds the full path for a GKE cluster
// Format: projects/{project}/locations/{location}/clusters/{cluster}
func BuildClusterPath(project, location, clusterName string) string {
	return fmt.Sprintf("projects/%s/locations/%s/clusters/%s", project, location, clusterName)
}

// BuildOperationPath builds the full path for a GKE operation
// Format: projects/{project}/locations/{location}/operations/{operation}
func BuildOperationPath(project, location, operationName string) string {
	return fmt.Sprintf("projects/%s/locations/%s/operations/%s", project, location, operationName)
}

// BuildNodePoolPath builds the full path for a GKE node pool
// Format: projects/{project}/locations/{location}/clusters/{cluster}/nodePools/{nodePool}
func BuildNodePoolPath(project, location, clusterName, nodePoolName string) string {
	return fmt.Sprintf("projects/%s/locations/%s/clusters/%s/nodePools/%s",
		project, location, clusterName, nodePoolName)
}

// ParseClusterPath parses a cluster native ID into its components
// Input: projects/{project}/locations/{location}/clusters/{cluster}
func ParseClusterPath(nativeID string) (project, location, clusterName string, err error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "clusters" {
		return "", "", "", fmt.Errorf("invalid cluster path format: %s (expected: projects/{project}/locations/{location}/clusters/{cluster})", nativeID)
	}

	return parts[1], parts[3], parts[5], nil
}

// ParseOperationPath parses an operation ID into its components
// Input: projects/{project}/locations/{location}/operations/{operation}
func ParseOperationPath(operationID string) (project, location, operationName string, err error) {
	parts := strings.Split(operationID, "/")
	if len(parts) != 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "operations" {
		return "", "", "", fmt.Errorf("invalid operation path format: %s (expected: projects/{project}/locations/{location}/operations/{operation})", operationID)
	}

	return parts[1], parts[3], parts[5], nil
}

// ParseNodePoolPath parses a node pool native ID into its components
// Input: projects/{project}/locations/{location}/clusters/{cluster}/nodePools/{nodePool}
func ParseNodePoolPath(nativeID string) (project, location, clusterName, nodePoolName string, err error) {
	parts := strings.Split(nativeID, "/")
	if len(parts) != 8 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "clusters" || parts[6] != "nodePools" {
		return "", "", "", "", fmt.Errorf("invalid node pool path format: %s (expected: projects/{project}/locations/{location}/clusters/{cluster}/nodePools/{nodePool})", nativeID)
	}

	return parts[1], parts[3], parts[5], parts[7], nil
}

// ExtractOperationNameFromSelfLink extracts the operation name from a GKE operation response
// The operation name from the API might be just the short name or the full path
func ExtractOperationNameFromSelfLink(opName string) string {
	// If it's already a short name (no slashes), return as-is
	if !strings.Contains(opName, "/") {
		return opName
	}

	// If it's a full path, extract the last component
	parts := strings.Split(opName, "/")
	return parts[len(parts)-1]
}

// ClusterPathComponents holds the parsed components of a cluster path
type ClusterPathComponents struct {
	Project     string
	Location    string
	ClusterName string
}

// ToNativeID converts the components back to a native ID
func (c *ClusterPathComponents) ToNativeID() string {
	return BuildClusterPath(c.Project, c.Location, c.ClusterName)
}

// ToParentPath returns the parent path
func (c *ClusterPathComponents) ToParentPath() string {
	return BuildParentPath(c.Project, c.Location)
}

// ToOperationPath builds an operation path with the given operation name
func (c *ClusterPathComponents) ToOperationPath(operationName string) string {
	return BuildOperationPath(c.Project, c.Location, operationName)
}

// NewClusterPathComponents creates a new ClusterPathComponents from a native ID
func NewClusterPathComponents(nativeID string) (*ClusterPathComponents, error) {
	project, location, clusterName, err := ParseClusterPath(nativeID)
	if err != nil {
		return nil, err
	}

	return &ClusterPathComponents{
		Project:     project,
		Location:    location,
		ClusterName: clusterName,
	}, nil
}

// ExtractClusterNameFromTargetLink extracts cluster information from operation target link
// Input: https://container.googleapis.com/v1/projects/{project}/locations/{location}/clusters/{cluster}
// Output: ClusterPathComponents
func ExtractClusterNameFromTargetLink(targetLink string) (*ClusterPathComponents, error) {
	if targetLink == "" {
		return nil, fmt.Errorf("target link is empty")
	}

	// Convert selfLink to nativeID
	nativeID := utils.SelfLinkToNativeID(targetLink)

	// Parse the native ID
	return NewClusterPathComponents(nativeID)
}

// OperationPathComponents holds the parsed components of an operation path
type OperationPathComponents struct {
	Project       string
	Location      string
	OperationName string
}

// ToRequestID converts the components to a full operation path (request ID)
func (o *OperationPathComponents) ToRequestID() string {
	return BuildOperationPath(o.Project, o.Location, o.OperationName)
}

// ToParentPath returns the parent path
func (o *OperationPathComponents) ToParentPath() string {
	return BuildParentPath(o.Project, o.Location)
}

// NewOperationPathComponents creates a new OperationPathComponents from a request ID
func NewOperationPathComponents(requestID string) (*OperationPathComponents, error) {
	project, location, operationName, err := ParseOperationPath(requestID)
	if err != nil {
		return nil, err
	}

	return &OperationPathComponents{
		Project:       project,
		Location:      location,
		OperationName: operationName,
	}, nil
}

// IsRegionalLocation checks if a location is a region (not a zone)
// Zones have format: {region}-{zone} (e.g., us-central1-a)
// Regions have format: {region} (e.g., us-central1)
func IsRegionalLocation(location string) bool {
	// Count dashes - zones have 2 dashes, regions have 1
	return strings.Count(location, "-") == 1
}

// ExtractRegionFromZone extracts the region from a zone
// Input: us-central1-a
// Output: us-central1
func ExtractRegionFromZone(zone string) string {
	parts := strings.Split(zone, "-")
	if len(parts) >= 2 {
		return strings.Join(parts[:2], "-")
	}
	return zone
}

// NormalizeLocation ensures the location is in the correct format
// If it's a zone, returns the region; otherwise returns as-is
func NormalizeLocation(location string, preferRegional bool) string {
	if preferRegional && !IsRegionalLocation(location) {
		return ExtractRegionFromZone(location)
	}
	return location
}

// BuildNodePoolParentPath builds the parent path for a node pool
// Format: projects/{project}/locations/{location}/clusters/{cluster}
func BuildNodePoolParentPath(project, location, cluster string) string {
	return fmt.Sprintf("projects/%s/locations/%s/clusters/%s", project, location, cluster)
}

// ExtractNodePoolNameFromTargetLink extracts node pool information from operation target link
// Input: https://container.googleapis.com/v1/projects/{project}/locations/{location}/clusters/{cluster}/nodePools/{nodePool}
func ExtractNodePoolNameFromTargetLink(targetLink string) (*NodePoolPathComponents, error) {
	if targetLink == "" {
		return nil, fmt.Errorf("target link is empty")
	}

	// Convert selfLink to nativeID
	nativeID := utils.SelfLinkToNativeID(targetLink)

	// Parse the native ID
	return NewNodePoolPathComponents(nativeID)
}

// NodePoolPathComponents holds the parsed components of a node pool path
type NodePoolPathComponents struct {
	Project      string
	Location     string
	ClusterName  string
	NodePoolName string
}

// ToNativeID converts the components back to a native ID
func (n *NodePoolPathComponents) ToNativeID() string {
	return BuildNodePoolPath(n.Project, n.Location, n.ClusterName, n.NodePoolName)
}

// ToParentPath returns the parent path
func (c *NodePoolPathComponents) ToParentPath() string {
	return BuildParentPath(c.Project, c.Location)
}

// ToOperationPath builds an operation path with the given operation name
func (c *NodePoolPathComponents) ToOperationPath(operationName string) string {
	return BuildOperationPath(c.Project, c.Location, operationName)
}

// ToClusterNativeID returns the cluster's native ID
func (n *NodePoolPathComponents) ToClusterNativeID() string {
	return BuildClusterPath(n.Project, n.Location, n.ClusterName)
}

// NewNodePoolPathComponents creates a new NodePoolPathComponents from a native ID
func NewNodePoolPathComponents(nativeID string) (*NodePoolPathComponents, error) {
	project, location, clusterName, nodePoolName, err := ParseNodePoolPath(nativeID)
	if err != nil {
		return nil, err
	}

	return &NodePoolPathComponents{
		Project:      project,
		Location:     location,
		ClusterName:  clusterName,
		NodePoolName: nodePoolName,
	}, nil
}
