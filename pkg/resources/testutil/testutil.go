// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/require"
)

// StatusChecker defines the interface for checking operation status
type StatusChecker interface {
	Status(ctx context.Context, request *resource.StatusRequest) (*resource.StatusResult, error)
}

// PollConfig configures the polling behavior
type PollConfig struct {
	MaxAttempts   int
	CheckInterval time.Duration
	ResourceType  string
	OperationName string // "Create", "Delete", "Update" for better logging
}

// DefaultPollConfig returns sensible defaults for polling
func DefaultPollConfig() PollConfig {
	return PollConfig{
		MaxAttempts:   30,
		CheckInterval: 2 * time.Second,
		OperationName: "Operation",
	}
}

// PollConfigBuilder provides a fluent API for building PollConfig
type PollConfigBuilder struct {
	config PollConfig
}

// NewPollConfig creates a new PollConfigBuilder with defaults
func NewPollConfig() *PollConfigBuilder {
	return &PollConfigBuilder{
		config: DefaultPollConfig(),
	}
}

// WithMaxAttempts sets the maximum number of polling attempts
func (b *PollConfigBuilder) WithMaxAttempts(attempts int) *PollConfigBuilder {
	b.config.MaxAttempts = attempts
	return b
}

// WithCheckInterval sets the interval between polling attempts
func (b *PollConfigBuilder) WithCheckInterval(interval time.Duration) *PollConfigBuilder {
	b.config.CheckInterval = interval
	return b
}

// WithResourceType sets the resource type
func (b *PollConfigBuilder) WithResourceType(resourceType string) *PollConfigBuilder {
	b.config.ResourceType = resourceType
	return b
}

// WithOperationName sets the operation name for logging
func (b *PollConfigBuilder) WithOperationName(name string) *PollConfigBuilder {
	b.config.OperationName = name
	return b
}

// ForCreate configures for a create operation (default settings)
func (b *PollConfigBuilder) ForCreate() *PollConfigBuilder {
	b.config.OperationName = "Create"
	return b
}

// ForDelete configures for a delete operation (default settings)
func (b *PollConfigBuilder) ForDelete() *PollConfigBuilder {
	b.config.OperationName = "Delete"
	return b
}

// ForUpdate configures for an update operation (default settings)
func (b *PollConfigBuilder) ForUpdate() *PollConfigBuilder {
	b.config.OperationName = "Update"
	return b
}

// ForLongRunningCreate configures for long-running create operations (e.g., GKE clusters)
func (b *PollConfigBuilder) ForLongRunningCreate() *PollConfigBuilder {
	b.config.OperationName = "Create"
	b.config.MaxAttempts = 200 // ~20 minutes with 6s intervals
	b.config.CheckInterval = 6 * time.Second
	return b
}

// ForLongRunningDelete configures for long-running delete operations
func (b *PollConfigBuilder) ForLongRunningDelete() *PollConfigBuilder {
	b.config.OperationName = "Delete"
	b.config.MaxAttempts = 200 // ~20 minutes with 6s intervals
	b.config.CheckInterval = 6 * time.Second
	return b
}

// Build returns the final PollConfig
func (b *PollConfigBuilder) Build() PollConfig {
	return b.config
}

// PollUntilComplete polls the status until the operation completes or times out
func PollUntilComplete(
	t *testing.T,
	ctx context.Context,
	checker StatusChecker,
	requestID string,
	targetConfig json.RawMessage,
	config PollConfig,
) (*resource.StatusResult, error) {
	t.Helper()

	if config.MaxAttempts == 0 {
		config.MaxAttempts = 30
	}
	if config.CheckInterval == 0 {
		config.CheckInterval = 2 * time.Second
	}

	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		time.Sleep(config.CheckInterval)

		statusReq := &resource.StatusRequest{
			RequestID:    requestID,
			ResourceType: config.ResourceType,
			TargetConfig: targetConfig,
		}

		statusResult, err := checker.Status(ctx, statusReq)
		require.NoError(t, err, "%s status check should not return error", config.OperationName)
		require.NotNil(t, statusResult, "%s status result should not be nil", config.OperationName)
		require.NotNil(t, statusResult.ProgressResult, "%s progress result should not be nil", config.OperationName)

		t.Logf("%s status check attempt %d/%d: %s (status: %s)",
			config.OperationName,
			attempt+1,
			config.MaxAttempts,
			statusResult.ProgressResult.StatusMessage,
			statusResult.ProgressResult.OperationStatus)

		switch statusResult.ProgressResult.OperationStatus {
		case resource.OperationStatusSuccess:
			t.Logf("%s completed successfully with native ID: %s",
				config.OperationName,
				statusResult.ProgressResult.NativeID)
			return statusResult, nil

		case resource.OperationStatusFailure:
			return statusResult, fmt.Errorf("%s operation failed: %s (error code: %s)",
				config.OperationName,
				statusResult.ProgressResult.StatusMessage,
				statusResult.ProgressResult.ErrorCode)

		case resource.OperationStatusInProgress:
			// Continue polling
			if attempt == config.MaxAttempts-1 {
				return statusResult, fmt.Errorf("%s operation timed out after %d attempts",
					config.OperationName,
					config.MaxAttempts)
			}
		}
	}

	return nil, fmt.Errorf("%s operation timed out", config.OperationName)
}

// WaitForCreate is a convenience wrapper for Create operations
func WaitForCreate(
	t *testing.T,
	ctx context.Context,
	checker StatusChecker,
	createResult *resource.CreateResult,
	targetConfig json.RawMessage,
	resourceType string,
) (*resource.StatusResult, error) {
	t.Helper()

	config := NewPollConfig().
		ForCreate().
		WithResourceType(resourceType).
		Build()

	return PollUntilComplete(t, ctx, checker, createResult.ProgressResult.RequestID, targetConfig, config)
}

// WaitForCreateWithConfig is a convenience wrapper with custom config
func WaitForCreateWithConfig(
	t *testing.T,
	ctx context.Context,
	checker StatusChecker,
	createResult *resource.CreateResult,
	targetConfig json.RawMessage,
	resourceType string,
	pollConfig PollConfig,
) (*resource.StatusResult, error) {
	t.Helper()

	if pollConfig.ResourceType == "" {
		pollConfig.ResourceType = resourceType
	}
	if pollConfig.OperationName == "" {
		pollConfig.OperationName = "Create"
	}

	return PollUntilComplete(t, ctx, checker, createResult.ProgressResult.RequestID, targetConfig, pollConfig)
}

// WaitForDelete is a convenience wrapper for Delete operations
func WaitForDelete(
	t *testing.T,
	ctx context.Context,
	checker StatusChecker,
	deleteResult *resource.DeleteResult,
	targetConfig json.RawMessage,
	resourceType string,
) (*resource.StatusResult, error) {
	t.Helper()

	config := NewPollConfig().
		ForDelete().
		WithResourceType(resourceType).
		Build()

	return PollUntilComplete(t, ctx, checker, deleteResult.ProgressResult.RequestID, targetConfig, config)
}

// WaitForDeleteWithConfig is a convenience wrapper with custom config
func WaitForDeleteWithConfig(
	t *testing.T,
	ctx context.Context,
	checker StatusChecker,
	deleteResult *resource.DeleteResult,
	targetConfig json.RawMessage,
	resourceType string,
	pollConfig PollConfig,
) (*resource.StatusResult, error) {
	t.Helper()

	if pollConfig.ResourceType == "" {
		pollConfig.ResourceType = resourceType
	}
	if pollConfig.OperationName == "" {
		pollConfig.OperationName = "Delete"
	}

	return PollUntilComplete(t, ctx, checker, deleteResult.ProgressResult.RequestID, targetConfig, pollConfig)
}

// WaitForUpdate is a convenience wrapper for Update operations
func WaitForUpdate(
	t *testing.T,
	ctx context.Context,
	checker StatusChecker,
	updateResult *resource.UpdateResult,
	targetConfig json.RawMessage,
	resourceType string,
) (*resource.StatusResult, error) {
	t.Helper()

	config := NewPollConfig().
		ForUpdate().
		WithResourceType(resourceType).
		Build()

	return PollUntilComplete(t, ctx, checker, updateResult.ProgressResult.RequestID, targetConfig, config)
}

// WaitForUpdateWithConfig is a convenience wrapper with custom config
func WaitForUpdateWithConfig(
	t *testing.T,
	ctx context.Context,
	checker StatusChecker,
	updateResult *resource.UpdateResult,
	targetConfig json.RawMessage,
	resourceType string,
	pollConfig PollConfig,
) (*resource.StatusResult, error) {
	t.Helper()

	if pollConfig.ResourceType == "" {
		pollConfig.ResourceType = resourceType
	}
	if pollConfig.OperationName == "" {
		pollConfig.OperationName = "Update"
	}

	return PollUntilComplete(t, ctx, checker, updateResult.ProgressResult.RequestID, targetConfig, pollConfig)
}

// WaitForStatus is a generic wrapper for checking status of any operation
func WaitForStatus(
	t *testing.T,
	ctx context.Context,
	checker StatusChecker,
	statusReq *resource.StatusRequest,
) (*resource.StatusResult, error) {
	t.Helper()

	config := NewPollConfig().
		WithResourceType(statusReq.ResourceType).
		Build()

	return PollUntilComplete(t, ctx, checker, statusReq.RequestID, statusReq.TargetConfig, config)
}
