// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
)

// ServiceNetworkingHelper provides utilities for managing Service Networking connections
type ServiceNetworkingHelper struct {
	Config *config.Config
}

// NewServiceNetworkingHelper creates a new Service Networking helper
func NewServiceNetworkingHelper(cfg *config.Config) *ServiceNetworkingHelper {
	return &ServiceNetworkingHelper{
		Config: cfg,
	}
}

// CreateGlobalAddressForVPCPeering creates a global address range for VPC peering
// This is required for Service Networking (Cloud SQL, etc.)
func (h *ServiceNetworkingHelper) CreateGlobalAddressForVPCPeering(
	ctx context.Context,
	addressName string,
	network string,
	prefixLength int,
) (string, error) {
	client, err := transport.NewClient(ctx, h.Config)
	if err != nil {
		return "", fmt.Errorf("failed to create transport client: %w", err)
	}

	// Build request body for global address with VPC_PEERING purpose
	body := map[string]interface{}{
		"name":         addressName,
		"purpose":      "VPC_PEERING",
		"addressType":  "INTERNAL",
		"prefixLength": prefixLength,
		"network":      network,
	}

	url := fmt.Sprintf("https://compute.googleapis.com/compute/v1/projects/%s/global/addresses",
		h.Config.Project)

	// Create the address
	response, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "POST",
		URL:    url,
		Body:   body,
	})
	if err != nil {
		// Check if address already exists
		if strings.Contains(err.Error(), "alreadyExists") || strings.Contains(err.Error(), "already exists") {
			return addressName, nil
		}
		return "", fmt.Errorf("failed to create global address: %w", err)
	}

	// Wait for operation to complete
	if opURL, err := transport.ExtractOperationURL(response.Body, ""); err == nil {
		if err := h.waitForGlobalOperation(ctx, client, opURL); err != nil {
			return "", fmt.Errorf("failed waiting for address creation: %w", err)
		}
	}

	return addressName, nil
}

// EstablishServiceNetworkingConnection creates a peering connection for Service Networking
func (h *ServiceNetworkingHelper) EstablishServiceNetworkingConnection(
	ctx context.Context,
	network string,
	reservedRanges []string,
) error {
	client, err := transport.NewClient(ctx, h.Config)
	if err != nil {
		return fmt.Errorf("failed to create transport client: %w", err)
	}

	// Build request body for service networking connection
	body := map[string]interface{}{
		"network":                network,
		"reservedPeeringRanges": reservedRanges,
	}

	// Service Networking API path: POST /v1/{parent=services/*}/connections
	// parent should be "services/servicenetworking.googleapis.com"
	url := "https://servicenetworking.googleapis.com/v1/services/servicenetworking.googleapis.com/connections"

	// Create the connection
	response, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "POST",
		URL:    url,
		Body:   body,
	})
	if err != nil {
		// Check if connection already exists
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "ALREADY_EXISTS") {
			return nil
		}
		return fmt.Errorf("failed to create service networking connection: %w", err)
	}

	// Wait for operation to complete (Service Networking operations)
	if opName, ok := response.Body["name"].(string); ok {
		if err := h.waitForServiceNetworkingOperation(ctx, client, opName); err != nil {
			return fmt.Errorf("failed waiting for service networking connection: %w", err)
		}
	}

	return nil
}

// CheckServiceNetworkingConnection checks if a service networking connection exists
func (h *ServiceNetworkingHelper) CheckServiceNetworkingConnection(
	ctx context.Context,
	network string,
) (bool, error) {
	client, err := transport.NewClient(ctx, h.Config)
	if err != nil {
		return false, fmt.Errorf("failed to create transport client: %w", err)
	}

	// List connections for the network - parent parameter required
	url := fmt.Sprintf("https://servicenetworking.googleapis.com/v1/services/servicenetworking.googleapis.com/connections?parent=%s", network)

	response, err := client.SendRequest(ctx, transport.RequestOptions{
		Method: "GET",
		URL:    url,
	})
	if err != nil {
		return false, nil // Connection doesn't exist
	}

	// Check if there are any connections
	if connections, ok := response.Body["connections"].([]interface{}); ok && len(connections) > 0 {
		return true, nil
	}

	return false, nil
}

// waitForGlobalOperation waits for a global compute operation to complete
func (h *ServiceNetworkingHelper) waitForGlobalOperation(
	ctx context.Context,
	client *transport.Client,
	opURL string,
) error {
	// Extract operation name from URL
	parts := strings.Split(opURL, "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid operation URL: %s", opURL)
	}
	opName := parts[len(parts)-1]

	url := fmt.Sprintf("https://compute.googleapis.com/compute/v1/projects/%s/global/operations/%s",
		h.Config.Project, opName)

	// Poll until done
	for i := 0; i < 60; i++ {
		response, err := client.SendRequest(ctx, transport.RequestOptions{
			Method: "GET",
			URL:    url,
		})
		if err != nil {
			return err
		}

		status, _ := response.Body["status"].(string)
		if status == "DONE" {
			// Check for errors
			if errorObj, ok := response.Body["error"].(map[string]interface{}); ok {
				if errors, ok := errorObj["errors"].([]interface{}); ok && len(errors) > 0 {
					if firstErr, ok := errors[0].(map[string]interface{}); ok {
						if msg, ok := firstErr["message"].(string); ok {
							return fmt.Errorf("operation failed: %s", msg)
						}
					}
				}
			}
			return nil
		}

		// Wait before next poll
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	return fmt.Errorf("operation timed out")
}

// waitForServiceNetworkingOperation waits for a Service Networking operation to complete
func (h *ServiceNetworkingHelper) waitForServiceNetworkingOperation(
	ctx context.Context,
	client *transport.Client,
	opName string,
) error {
	url := fmt.Sprintf("https://servicenetworking.googleapis.com/v1/%s", opName)

	// Poll until done
	for i := 0; i < 60; i++ {
		response, err := client.SendRequest(ctx, transport.RequestOptions{
			Method: "GET",
			URL:    url,
		})
		if err != nil {
			return err
		}

		done, _ := response.Body["done"].(bool)
		if done {
			// Check for errors
			if errorObj, ok := response.Body["error"].(map[string]interface{}); ok {
				if msg, ok := errorObj["message"].(string); ok {
					return fmt.Errorf("operation failed: %s", msg)
				}
			}
			return nil
		}

		// Wait before next poll
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}

	return fmt.Errorf("operation timed out")
}

// SetupServiceNetworkingForSQL sets up Service Networking for Cloud SQL
// This is a convenience method that combines address creation and peering establishment
func (h *ServiceNetworkingHelper) SetupServiceNetworkingForSQL(
	ctx context.Context,
	network string,
	addressName string,
	prefixLength int,
) error {
	// Check if connection already exists
	exists, err := h.CheckServiceNetworkingConnection(ctx, network)
	if err != nil {
		return fmt.Errorf("failed to check existing connection: %w", err)
	}
	if exists {
		return nil // Already set up
	}

	// Create global address for VPC peering
	_, err = h.CreateGlobalAddressForVPCPeering(ctx, addressName, network, prefixLength)
	if err != nil {
		return fmt.Errorf("failed to create address: %w", err)
	}

	// Establish service networking connection
	err = h.EstablishServiceNetworkingConnection(ctx, network, []string{addressName})
	if err != nil {
		return fmt.Errorf("failed to establish connection: %w", err)
	}

	return nil
}
