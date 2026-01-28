package utils

import (
	"fmt"
	"strings"
)

// OperationScope represents the scope of a GCP operation
type OperationScope string

const (
	OperationScopeGlobal OperationScope = "global"
	OperationScopeRegion OperationScope = "region"
	OperationScopeZone   OperationScope = "zone"
)

// ParsedOperationID contains the parsed components of a GCP operation ID
type ParsedOperationID struct {
	Project       string
	Scope         OperationScope
	Region        string
	OperationName string
}

// ParseOperationID parses a GCP operation ID into its components.
// Supports the following formats:
// - Global: projects/{project}/global/operations/{operation}
// - Regional: projects/{project}/regions/{region}/operations/{operation}
// - Zonal: projects/{project}/zones/{zone}/operations/{operation}
func ParseOperationID(operationID string) (*ParsedOperationID, error) {
	parts := strings.Split(operationID, "/")

	// Global format: projects/{project}/global/operations/{operation}
	if len(parts) == 5 && parts[0] == "projects" && parts[2] == "global" && parts[3] == "operations" {
		return &ParsedOperationID{
			Project:       parts[1],
			Scope:         OperationScopeGlobal,
			Region:        "",
			OperationName: parts[4],
		}, nil
	}

	// Regional format: projects/{project}/regions/{region}/operations/{operation}
	// Zonal format: projects/{project}/zones/{zone}/operations/{operation}
	if len(parts) == 6 && parts[0] == "projects" && parts[2] == "regions" && parts[4] == "operations" {
		return &ParsedOperationID{
			Project:       parts[1],
			Scope:         OperationScopeRegion,
			Region:        parts[3],
			OperationName: parts[5],
		}, nil
	}

	return nil, fmt.Errorf("invalid operation ID format: %s (expected: projects/{project}/{global|regions/{region}|zones/{zone}}/operations/{operation})", operationID)
}

// IsGlobal returns true if the operation is global scope
func (p *ParsedOperationID) IsGlobal() bool {
	return p.Scope == OperationScopeGlobal
}

// IsRegional returns true if the operation is regional scope
func (p *ParsedOperationID) IsRegional() bool {
	return p.Scope == OperationScopeRegion
}

// IsZonal returns true if the operation is zonal scope
func (p *ParsedOperationID) IsZonal() bool {
	return p.Scope == OperationScopeZone
}

// String returns the operation ID as a string
func (p *ParsedOperationID) String() string {
	switch p.Scope {
	case OperationScopeGlobal:
		return fmt.Sprintf("projects/%s/global/operations/%s", p.Project, p.OperationName)
	case OperationScopeRegion:
		return fmt.Sprintf("projects/%s/regions/%s/operations/%s", p.Project, p.Region, p.OperationName)
	case OperationScopeZone:
		return fmt.Sprintf("projects/%s/zones/%s/operations/%s", p.Project, p.Region, p.OperationName)
	default:
		return ""
	}
}

// SelfLinkToNativeID converts a GCP selfLink URL to a native ID format.
//
// Examples:
//   - https://www.googleapis.com/compute/v1/projects/my-project/regions/us-central1/subnetworks/my-subnet
//     => projects/my-project/regions/us-central1/subnetworks/my-subnet
//   - https://container.googleapis.com/v1/projects/my-project/locations/us-central1/clusters/my-cluster
//     => projects/my-project/locations/us-central1/clusters/my-cluster
//   - https://www.googleapis.com/compute/v1/projects/my-project/global/networks/my-network
//     => projects/my-project/global/networks/my-network
func SelfLinkToNativeID(selfLink string) string {
	if selfLink == "" {
		return ""
	}

	// Remove the base URL prefix
	// Common prefixes:
	// - https://www.googleapis.com/compute/v1/
	// - https://container.googleapis.com/v1/
	// - https://www.googleapis.com/storage/v1/

	// Find "projects/" in the URL
	idx := strings.Index(selfLink, "projects/")
	if idx == -1 {
		// If no "projects/" found, return as-is
		return selfLink
	}

	// Return everything from "projects/" onwards
	return selfLink[idx:]
}

// NativeIDToSelfLink converts a native ID to a selfLink URL format.
// This is the reverse of SelfLinkToNativeID.
//
// Examples:
//   - projects/my-project/regions/us-central1/subnetworks/my-subnet
//     => https://www.googleapis.com/compute/v1/projects/my-project/regions/us-central1/subnetworks/my-subnet
//   - projects/my-project/locations/us-central1/clusters/my-cluster
//     => https://container.googleapis.com/v1/projects/my-project/locations/us-central1/clusters/my-cluster
func NativeIDToSelfLink(nativeID, service string) string {
	if nativeID == "" {
		return ""
	}

	// If it already looks like a full URL, return as-is
	if strings.HasPrefix(nativeID, "https://") {
		return nativeID
	}

	// Determine the base URL based on the service and resource type
	var baseURL string
	switch {
	case strings.Contains(nativeID, "/locations/") && strings.Contains(nativeID, "/clusters/"):
		// Container/GKE resources use /locations/
		baseURL = "https://container.googleapis.com/v1/"
	case strings.Contains(nativeID, "/regions/") || strings.Contains(nativeID, "/zones/") || strings.Contains(nativeID, "/global/"):
		// Compute resources
		baseURL = "https://www.googleapis.com/compute/v1/"
	default:
		// Default to compute
		baseURL = "https://www.googleapis.com/compute/v1/"
	}

	// If a specific service is provided, use it
	if service != "" {
		switch service {
		case "compute":
			baseURL = "https://www.googleapis.com/compute/v1/"
		case "container":
			baseURL = "https://container.googleapis.com/v1/"
		case "storage":
			baseURL = "https://www.googleapis.com/storage/v1/"
		}
	}

	return baseURL + nativeID
}
