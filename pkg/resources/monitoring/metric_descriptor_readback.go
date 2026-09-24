// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package monitoring

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const (
	metricDescriptorCallbackTimeout = 10 * time.Second
	metricDescriptorMarkerNamespace = "formae:gcp:metric-descriptor:create-readiness:"
	metricDescriptorMarkerPrefix    = metricDescriptorMarkerNamespace + "v1:"
	metricDescriptorMarkerMaxLength = 4096
)

type metricDescriptorReadinessMarker struct {
	NativeID     string `json:"nativeID"`
	DeadlineUnix int64  `json:"deadlineUnix"`
}

type metricDescriptorProvisioner struct {
	*base.BaseResource

	// These private call seams keep the protocol deterministic in unit tests.
	// Production always takes the BaseResource and time.Now fallbacks.
	createCall func(context.Context, *resource.CreateRequest) (*resource.CreateResult, error)
	readCall   func(context.Context, *resource.ReadRequest) (*resource.ReadResult, error)
	now        func() time.Time
}

// registerMetricDescriptorReadback re-registers Create and Status after the
// generic Monitoring registrations. Create reports provider acceptance as a
// durable pending operation; marked Status requests perform one readiness read.
func registerMetricDescriptorReadback() {
	registry.Register(MetricDescriptorResourceType,
		[]resource.Operation{resource.OperationCreate, resource.OperationCheckStatus},
		func(cfg *config.Config) prov.Provisioner {
			def := monitoringRegistry.Definitions[MetricDescriptorResourceType]
			return &metricDescriptorProvisioner{
				BaseResource: &base.BaseResource{
					Config:              cfg,
					APIConfig:           MonitoringAPI,
					OperationConfig:     MonitoringOperations,
					ResourceConfig:      def.ResourceConfig,
					NativeIDConfig:      MonitoringMetricDescriptorNativeID,
					RequestTransformer:  def.RequestTransformer,
					ResponseTransformer: def.ResponseTransformer,
				},
			}
		})
}

func (m *metricDescriptorProvisioner) Create(
	ctx context.Context,
	request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	callCtx, cancel := context.WithTimeout(ctx, metricDescriptorCallbackTimeout)
	defer cancel()

	result, err := m.callCreate(callCtx, request)
	if progress := createProgress(result); progress != nil &&
		progress.OperationStatus == resource.OperationStatusSuccess {
		if _, parseErr := metricDescriptorTypeFromNativeID(progress.NativeID); parseErr != nil {
			return metricDescriptorCreateFailure(result,
				"metric descriptor creation succeeded without a usable native identity"), nil
		}
		marker, markerErr := encodeMetricDescriptorReadinessMarker(progress.NativeID,
			m.currentTime().Add(transport.DefaultOperationTimeout))
		if markerErr != nil {
			return metricDescriptorCreateFailure(result,
				fmt.Sprintf("failed to record metric descriptor readiness: %v", markerErr)), nil
		}

		// A successful provider result proves acceptance even when cancellation
		// raced with response handling. Keep its identity and observed properties.
		progress.Operation = resource.OperationCreate
		progress.OperationStatus = resource.OperationStatusInProgress
		progress.RequestID = marker
		progress.ErrorCode = resource.OperationErrorCodeNotSet
		progress.StatusMessage = "Metric descriptor creation accepted; waiting for it to become readable"
		return result, nil
	}

	// A cancelled bounded call with no successful provider result has an
	// unknown outcome. A recoverable failure would cause the POST to be replayed.
	if callCtx.Err() != nil {
		return metricDescriptorCreateFailure(result,
			"Metric descriptor creation outcome is unknown because the request deadline expired or was cancelled"), nil
	}
	return result, err
}

func (m *metricDescriptorProvisioner) Status(
	ctx context.Context,
	request *resource.StatusRequest,
) (*resource.StatusResult, error) {
	if request == nil || !strings.HasPrefix(request.RequestID, metricDescriptorMarkerNamespace) {
		result, err := m.BaseResource.Status(ctx, request)
		if err == nil && request != nil && result != nil && result.ProgressResult != nil &&
			result.ProgressResult.NativeID == "" {
			result.ProgressResult.NativeID = request.NativeID
		}
		return result, err
	}

	marker, err := parseMetricDescriptorReadinessMarker(request.RequestID)
	if err != nil {
		return metricDescriptorStatusFailure(request, resource.OperationErrorCodeGeneralServiceException,
			fmt.Sprintf("Invalid metric descriptor readiness marker: %v", err)), nil
	}
	if marker.NativeID != request.NativeID {
		return metricDescriptorStatusFailure(request, resource.OperationErrorCodeGeneralServiceException,
			"Metric descriptor readiness marker does not match the requested native identity"), nil
	}
	expectedType, err := metricDescriptorTypeFromNativeID(marker.NativeID)
	if err != nil {
		return metricDescriptorStatusFailure(request, resource.OperationErrorCodeGeneralServiceException,
			fmt.Sprintf("Invalid metric descriptor native identity: %v", err)), nil
	}
	deadline := time.Unix(marker.DeadlineUnix, 0)
	if !m.currentTime().Before(deadline) {
		return metricDescriptorReadinessExpired(request), nil
	}

	callCtx, cancel := context.WithTimeout(ctx, metricDescriptorCallbackTimeout)
	defer cancel()
	readResult, readErr := m.callRead(callCtx, &resource.ReadRequest{
		NativeID:     request.NativeID,
		ResourceType: request.ResourceType,
		TargetConfig: request.TargetConfig,
	})
	if readErr != nil || readResult == nil {
		return m.metricDescriptorPendingOrExpired(request, deadline,
			"Metric descriptor creation was accepted; readiness could not yet be confirmed"), nil
	}

	switch readResult.ErrorCode {
	case resource.OperationErrorCodeAccessDenied, resource.OperationErrorCodeInvalidRequest:
		return metricDescriptorStatusFailure(request, readResult.ErrorCode,
			"Metric descriptor readiness read returned a terminal error"), nil
	case resource.OperationErrorCodeNotSet:
		// Validate the successful read below.
	default:
		return m.metricDescriptorPendingOrExpired(request, deadline,
			fmt.Sprintf("Metric descriptor creation was accepted; readiness read returned %s", readResult.ErrorCode)), nil
	}

	if err := validateMetricDescriptorReadProperties(readResult.Properties, expectedType); err != nil {
		return metricDescriptorStatusFailure(request, resource.OperationErrorCodeGeneralServiceException,
			fmt.Sprintf("Metric descriptor readiness read returned an invalid descriptor: %v", err)), nil
	}
	return &resource.StatusResult{ProgressResult: &resource.ProgressResult{
		Operation:          resource.OperationCreate,
		OperationStatus:    resource.OperationStatusSuccess,
		NativeID:           request.NativeID,
		RequestID:          request.RequestID,
		ResourceProperties: json.RawMessage(readResult.Properties),
		StatusMessage:      "Metric descriptor is readable",
	}}, nil
}

func (m *metricDescriptorProvisioner) callCreate(
	ctx context.Context, request *resource.CreateRequest,
) (*resource.CreateResult, error) {
	if m.createCall != nil {
		return m.createCall(ctx, request)
	}
	return m.BaseResource.Create(ctx, request)
}

func (m *metricDescriptorProvisioner) callRead(
	ctx context.Context, request *resource.ReadRequest,
) (*resource.ReadResult, error) {
	if m.readCall != nil {
		return m.readCall(ctx, request)
	}
	return m.Read(ctx, request)
}

func (m *metricDescriptorProvisioner) currentTime() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

func (m *metricDescriptorProvisioner) metricDescriptorPendingOrExpired(
	request *resource.StatusRequest, deadline time.Time, message string,
) *resource.StatusResult {
	if !m.currentTime().Before(deadline) {
		return metricDescriptorReadinessExpired(request)
	}
	return &resource.StatusResult{ProgressResult: &resource.ProgressResult{
		Operation:       resource.OperationCreate,
		OperationStatus: resource.OperationStatusInProgress,
		NativeID:        request.NativeID,
		RequestID:       request.RequestID,
		StatusMessage:   message,
	}}
}

func createProgress(result *resource.CreateResult) *resource.ProgressResult {
	if result == nil {
		return nil
	}
	return result.ProgressResult
}

func metricDescriptorCreateFailure(result *resource.CreateResult, message string) *resource.CreateResult {
	if result == nil {
		result = &resource.CreateResult{}
	}
	if result.ProgressResult == nil {
		result.ProgressResult = &resource.ProgressResult{}
	}
	progress := result.ProgressResult
	progress.Operation = resource.OperationCreate
	progress.OperationStatus = resource.OperationStatusFailure
	progress.RequestID = ""
	progress.ErrorCode = resource.OperationErrorCodeGeneralServiceException
	progress.StatusMessage = message
	return result
}

func metricDescriptorStatusFailure(
	request *resource.StatusRequest, code resource.OperationErrorCode, message string,
) *resource.StatusResult {
	return &resource.StatusResult{ProgressResult: &resource.ProgressResult{
		Operation:       resource.OperationCreate,
		OperationStatus: resource.OperationStatusFailure,
		NativeID:        request.NativeID,
		RequestID:       request.RequestID,
		ErrorCode:       code,
		StatusMessage:   message,
	}}
}

func metricDescriptorReadinessExpired(request *resource.StatusRequest) *resource.StatusResult {
	return metricDescriptorStatusFailure(request, resource.OperationErrorCodeGeneralServiceException,
		"Metric descriptor creation was accepted but readiness was not confirmed before the deadline")
}

func encodeMetricDescriptorReadinessMarker(nativeID string, deadline time.Time) (string, error) {
	payload, err := json.Marshal(metricDescriptorReadinessMarker{
		NativeID:     nativeID,
		DeadlineUnix: deadline.Unix(),
	})
	if err != nil {
		return "", err
	}
	return metricDescriptorMarkerPrefix + base64.RawURLEncoding.EncodeToString(payload), nil
}

func parseMetricDescriptorReadinessMarker(requestID string) (metricDescriptorReadinessMarker, error) {
	if len(requestID) > metricDescriptorMarkerMaxLength {
		return metricDescriptorReadinessMarker{}, fmt.Errorf("marker exceeds %d bytes", metricDescriptorMarkerMaxLength)
	}
	if !strings.HasPrefix(requestID, metricDescriptorMarkerPrefix) {
		return metricDescriptorReadinessMarker{}, fmt.Errorf("unsupported marker version")
	}
	encoded := strings.TrimPrefix(requestID, metricDescriptorMarkerPrefix)
	if encoded == "" {
		return metricDescriptorReadinessMarker{}, fmt.Errorf("marker payload is empty")
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return metricDescriptorReadinessMarker{}, fmt.Errorf("decode marker payload: %w", err)
	}
	var marker metricDescriptorReadinessMarker
	if err := json.Unmarshal(payload, &marker); err != nil {
		return metricDescriptorReadinessMarker{}, fmt.Errorf("decode marker JSON: %w", err)
	}
	if marker.NativeID == "" {
		return metricDescriptorReadinessMarker{}, fmt.Errorf("native identity is empty")
	}
	if marker.DeadlineUnix <= 0 {
		return metricDescriptorReadinessMarker{}, fmt.Errorf("deadline is missing")
	}
	if _, err := metricDescriptorTypeFromNativeID(marker.NativeID); err != nil {
		return metricDescriptorReadinessMarker{}, err
	}
	return marker, nil
}

func metricDescriptorTypeFromNativeID(nativeID string) (string, error) {
	pathCtx, err := parseMetricDescriptorNativeID(nativeID)
	if err != nil {
		return "", err
	}
	return pathCtx.ResourceName, nil
}

func validateMetricDescriptorReadProperties(properties, expectedType string) error {
	var observed map[string]interface{}
	if err := json.Unmarshal([]byte(properties), &observed); err != nil {
		return fmt.Errorf("decode properties: %w", err)
	}
	if len(observed) == 0 {
		return fmt.Errorf("properties are empty")
	}
	observedType, ok := observed["name"].(string)
	if !ok || observedType == "" {
		return fmt.Errorf("descriptor identity is missing")
	}
	if observedType != expectedType {
		return fmt.Errorf("descriptor identity %q does not match %q", observedType, expectedType)
	}
	return nil
}
