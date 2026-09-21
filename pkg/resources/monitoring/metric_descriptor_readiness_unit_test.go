// © 2026 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package monitoring

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/prov"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const readinessFixtureNativeID = "projects/test-project/metricDescriptors/custom.googleapis.com/formae/test"

// A successful POST is acceptance, not readiness; Create must return pending
// and leave readability checks to Status.
func TestMetricDescriptorCreateRemainsPendingUntilReadable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var posts, gets atomic.Int32
	const nativeID = "projects/test-project/metricDescriptors/custom.googleapis.com/formae/test"
	server := monitoringAuthenticatedServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			posts.Add(1)
			_, _ = fmt.Fprintf(w, `{"name":%q,"type":"custom.googleapis.com/formae/test","metricKind":"GAUGE","valueType":"DOUBLE"}`, nativeID)
		case http.MethodGet:
			gets.Add(1)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"message":"not propagated yet"}}`))
			cancel()
		default:
			t.Errorf("unexpected request method %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	provisioner := registry.Get(MetricDescriptorResourceType, resource.OperationCreate, &config.Config{Project: "test-project"}).(*metricDescriptorProvisioner)
	provisioner.APIConfig.BaseURL = server.URL + "/v3"
	result, err := provisioner.Create(ctx, &resource.CreateRequest{
		ResourceType: MetricDescriptorResourceType,
		TargetConfig: []byte(`{"Project":"test-project"}`),
		Properties:   []byte(`{"name":"custom.googleapis.com/formae/test","metricKind":"GAUGE","valueType":"DOUBLE"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.ProgressResult == nil {
		t.Fatal("successful POST lost its progress result")
	}
	progress := result.ProgressResult
	if progress.OperationStatus != resource.OperationStatusInProgress {
		t.Fatalf("unreadable descriptor reported %s, want InProgress; POSTs=%d GETs=%d", progress.OperationStatus, posts.Load(), gets.Load())
	}
	if progress.NativeID != nativeID || progress.RequestID == "" {
		t.Fatalf("pending creation must retain identity and resumable request ID: %#v", progress)
	}
	if posts.Load() != 1 || gets.Load() != 0 {
		t.Fatalf("Create must issue one POST and leave readiness reads to Status; POSTs=%d GETs=%d", posts.Load(), gets.Load())
	}
}

// Exercise actual registered status provisioners and transport. These are
// externally visible outcomes, not assertions about a helper's implementation.
func TestMetricDescriptorReadinessStatus(t *testing.T) {
	for _, tc := range []struct {
		name       string
		httpStatus int
		body       string
		status     resource.OperationStatus
		errorCode  resource.OperationErrorCode
	}{
		{"not propagated", 404, `{"error":{"code":404,"message":"not propagated"}}`, resource.OperationStatusInProgress, ""},
		{"throttled", 429, `{"error":{"code":429,"message":"rate limit"}}`, resource.OperationStatusInProgress, ""},
		{"server unavailable", 503, `{"error":{"code":503,"message":"unavailable"}}`, resource.OperationStatusInProgress, ""},
		{"permission denied", 403, `{"error":{"code":403,"message":"permission denied"}}`, resource.OperationStatusFailure, resource.OperationErrorCodeAccessDenied},
		{"invalid request", 400, `{"error":{"code":400,"message":"bad request"}}`, resource.OperationStatusFailure, resource.OperationErrorCodeInvalidRequest},
		{"empty response", 200, ``, resource.OperationStatusInProgress, ""},
		{"null response", 200, `null`, resource.OperationStatusFailure, resource.OperationErrorCodeGeneralServiceException},
		{"malformed response", 200, `{not-json`, resource.OperationStatusInProgress, ""},
		{"missing identity", 200, `{}`, resource.OperationStatusFailure, resource.OperationErrorCodeGeneralServiceException},
		{"wrong identity", 200, `{"type":"custom.googleapis.com/formae/other","metricKind":"GAUGE","valueType":"DOUBLE"}`, resource.OperationStatusFailure, resource.OperationErrorCodeGeneralServiceException},
		{"readable", 200, fmt.Sprintf(`{"name":%q,"type":"custom.googleapis.com/formae/test","metricKind":"GAUGE","valueType":"DOUBLE"}`, readinessFixtureNativeID), resource.OperationStatusSuccess, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gets, writes atomic.Int32
			server := monitoringAuthenticatedServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes.Add(1)
				}
				gets.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.httpStatus)
				_, _ = w.Write([]byte(tc.body))
			})
			request := readinessFixtureRequest(t, readinessFixtureNativeID, time.Now().Add(5*time.Minute))
			p := readinessFixtureStatus(t, server.URL)
			result, err := p.Status(context.Background(), request)
			if err != nil || result == nil || result.ProgressResult == nil {
				t.Fatalf("Status result=%#v err=%v", result, err)
			}
			progress := result.ProgressResult
			if progress.OperationStatus != tc.status || progress.ErrorCode != tc.errorCode {
				t.Fatalf("status=%s code=%s, want status=%s code=%s", progress.OperationStatus, progress.ErrorCode, tc.status, tc.errorCode)
			}
			if progress.Operation != resource.OperationCreate {
				t.Fatalf("marked Status operation=%s, want Create", progress.Operation)
			}
			if progress.NativeID != request.NativeID || progress.RequestID != request.RequestID {
				t.Fatal("Status changed or discarded the original operation identity/deadline")
			}
			if gets.Load() != 1 || writes.Load() != 0 {
				t.Fatalf("Status must perform exactly one GET: requests=%d writes=%d", gets.Load(), writes.Load())
			}
			if tc.status == resource.OperationStatusSuccess && !strings.Contains(string(progress.ResourceProperties), "GAUGE") {
				t.Fatalf("successful readiness lost observed properties: %s", progress.ResourceProperties)
			}
		})
	}
}

func TestMetricDescriptorReadinessIdentityAndExpiry(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*resource.StatusRequest)
	}{
		{"expired", func(r *resource.StatusRequest) {
			r.RequestID = readinessFixtureRequest(t, r.NativeID, time.Now().Add(-time.Minute)).RequestID
		}},
		{"different resource", func(r *resource.StatusRequest) { r.NativeID += "-other" }},
		{"malformed payload", func(r *resource.StatusRequest) { r.RequestID = "formae:gcp:metric-descriptor:create-readiness:v1:!" }},
		{"missing identity", func(r *resource.StatusRequest) {
			r.RequestID = readinessFixtureMarker(t, "", time.Now().Add(5*time.Minute))
		}},
		{"missing deadline", func(r *resource.StatusRequest) {
			r.RequestID = readinessFixtureMarker(t, r.NativeID, time.Unix(0, 0))
		}},
		{"oversized payload", func(r *resource.StatusRequest) {
			r.RequestID = "formae:gcp:metric-descriptor:create-readiness:v1:" + strings.Repeat("a", 8192)
		}},
		{"unknown version", func(r *resource.StatusRequest) { r.RequestID = strings.Replace(r.RequestID, ":v1:", ":v2:", 1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := monitoringAuthenticatedServer(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			})
			request := readinessFixtureRequest(t, readinessFixtureNativeID, time.Now().Add(5*time.Minute))
			tc.edit(request)
			result, err := readinessFixtureStatus(t, server.URL).Status(context.Background(), request)
			if err != nil || result == nil || result.ProgressResult == nil {
				t.Fatalf("Status result=%#v err=%v", result, err)
			}
			progress := result.ProgressResult
			if progress.OperationStatus != resource.OperationStatusFailure || resource.IsRecoverable(progress.ErrorCode) {
				t.Fatalf("invalid/expired marker must fail without inviting POST retry: %#v", progress)
			}
			if progress.Operation != resource.OperationCreate || progress.ErrorCode != resource.OperationErrorCodeGeneralServiceException {
				t.Fatalf("invalid/expired marker has wrong terminal classification: %#v", progress)
			}
			if requests.Load() != 0 {
				t.Fatalf("invalid/expired marker made %d API requests", requests.Load())
			}
		})
	}
}

func TestMetricDescriptorReadinessSurvivesProvisionerRecreation(t *testing.T) {
	var reads atomic.Int32
	server := monitoringAuthenticatedServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected mutation %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		if reads.Add(1) < 3 {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"message":"not propagated"}}`))
			return
		}
		_, _ = fmt.Fprintf(w, `{"name":%q,"type":"custom.googleapis.com/formae/test","metricKind":"GAUGE","valueType":"DOUBLE"}`, readinessFixtureNativeID)
	})
	request := readinessFixtureRequest(t, readinessFixtureNativeID, time.Now().Add(5*time.Minute))
	for _, want := range []resource.OperationStatus{resource.OperationStatusInProgress, resource.OperationStatusInProgress, resource.OperationStatusSuccess} {
		// registry.Get returns a new provisioner on every callback in production.
		result, err := readinessFixtureStatus(t, server.URL).Status(context.Background(), request)
		if err != nil || result == nil || result.ProgressResult == nil || result.ProgressResult.OperationStatus != want {
			t.Fatalf("Status=%#v err=%v, want %s", result, err, want)
		}
		if result.ProgressResult.RequestID != request.RequestID {
			t.Fatal("recreation reset the durable readiness deadline")
		}
		if result.ProgressResult.Operation != resource.OperationCreate {
			t.Fatalf("marked Status operation=%s, want Create", result.ProgressResult.Operation)
		}
	}
	if reads.Load() != 3 {
		t.Fatalf("got %d reads, want one per callback", reads.Load())
	}
}

func TestMetricDescriptorReadinessHonorsCallerDeadline(t *testing.T) {
	var requests atomic.Int32
	server := monitoringAuthenticatedServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	})
	request := readinessFixtureRequest(t, readinessFixtureNativeID, time.Now().Add(5*time.Minute))
	p := readinessFixtureStatus(t, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	result, err := p.Status(ctx, request)
	if err != nil || result == nil || result.ProgressResult == nil {
		t.Fatalf("Status result=%#v err=%v", result, err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Status ignored the caller deadline: %s", elapsed)
	}
	progress := result.ProgressResult
	if progress.OperationStatus != resource.OperationStatusInProgress || progress.RequestID != request.RequestID || progress.NativeID != request.NativeID {
		t.Fatalf("interrupted readiness check discarded the pending operation: %#v", progress)
	}
	if progress.Operation != resource.OperationCreate || progress.ErrorCode != "" {
		t.Fatalf("interrupted readiness check returned a replayable failure: %#v", progress)
	}
	if requests.Load() > 1 {
		t.Fatalf("one status callback made %d API calls", requests.Load())
	}
}

func TestMetricDescriptorCreateDeadlineIsTerminalOutcomeUnknown(t *testing.T) {
	var posts atomic.Int32
	server := monitoringAuthenticatedServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected request method %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		posts.Add(1)
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	})
	provisioner := registry.Get(MetricDescriptorResourceType, resource.OperationCreate, &config.Config{Project: "test-project"}).(*metricDescriptorProvisioner)
	provisioner.APIConfig.BaseURL = server.URL + "/v3"
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	result, err := provisioner.Create(ctx, readinessFixtureCreateRequest())
	if err != nil || result == nil || result.ProgressResult == nil {
		t.Fatalf("Create result=%#v err=%v", result, err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Create ignored the caller deadline: %s", elapsed)
	}
	progress := result.ProgressResult
	if progress.Operation != resource.OperationCreate || progress.OperationStatus != resource.OperationStatusFailure ||
		progress.ErrorCode != resource.OperationErrorCodeGeneralServiceException || resource.IsRecoverable(progress.ErrorCode) {
		t.Fatalf("unknown POST outcome must be terminal and nonrecoverable: %#v", progress)
	}
	if progress.NativeID != "" || progress.RequestID != "" {
		t.Fatalf("unknown POST outcome fabricated readiness identity: %#v", progress)
	}
	if posts.Load() != 1 {
		t.Fatalf("Create made %d POSTs, want one", posts.Load())
	}
}

func TestMetricDescriptorCreateGoErrorAfterCancellationIsTerminalOutcomeUnknown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	provisioner := &metricDescriptorProvisioner{
		BaseResource: &base.BaseResource{},
		createCall: func(callCtx context.Context, request *resource.CreateRequest) (*resource.CreateResult, error) {
			cancel()
			<-callCtx.Done()
			return nil, errors.New("request ended without a response")
		},
	}
	result, err := provisioner.Create(ctx, readinessFixtureCreateRequest())
	if err != nil || result == nil || result.ProgressResult == nil {
		t.Fatalf("Create result=%#v err=%v", result, err)
	}
	progress := result.ProgressResult
	if progress.Operation != resource.OperationCreate || progress.OperationStatus != resource.OperationStatusFailure ||
		progress.ErrorCode != resource.OperationErrorCodeGeneralServiceException || progress.NativeID != "" || progress.RequestID != "" {
		t.Fatalf("cancelled Go-error create has wrong outcome: %#v", progress)
	}
}

func TestMetricDescriptorCreatePreservesOrdinaryFailure(t *testing.T) {
	var posts atomic.Int32
	server := monitoringAuthenticatedServer(t, func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":400,"message":"bad descriptor"}}`))
	})
	provisioner := registry.Get(MetricDescriptorResourceType, resource.OperationCreate, &config.Config{Project: "test-project"}).(*metricDescriptorProvisioner)
	provisioner.APIConfig.BaseURL = server.URL + "/v3"
	result, err := provisioner.Create(context.Background(), readinessFixtureCreateRequest())
	if err != nil || result == nil || result.ProgressResult == nil {
		t.Fatalf("Create result=%#v err=%v", result, err)
	}
	progress := result.ProgressResult
	if progress.Operation != resource.OperationCreate || progress.OperationStatus != resource.OperationStatusFailure ||
		progress.ErrorCode != resource.OperationErrorCodeInvalidRequest || progress.RequestID != "" {
		t.Fatalf("ordinary create failure changed classification: %#v", progress)
	}
	if posts.Load() != 1 {
		t.Fatalf("Create made %d POSTs, want one", posts.Load())
	}
}

func TestMetricDescriptorCreateSuccessWinsConcurrentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	provisioner := &metricDescriptorProvisioner{
		BaseResource: &base.BaseResource{},
		createCall: func(context.Context, *resource.CreateRequest) (*resource.CreateResult, error) {
			cancel()
			return &resource.CreateResult{ProgressResult: &resource.ProgressResult{
				Operation: resource.OperationCreate, OperationStatus: resource.OperationStatusSuccess,
				NativeID: readinessFixtureNativeID, ResourceProperties: json.RawMessage(`{"name":"custom.googleapis.com/formae/test"}`),
			}}, nil
		},
	}
	result, err := provisioner.Create(ctx, readinessFixtureCreateRequest())
	if err != nil || result == nil || result.ProgressResult == nil {
		t.Fatalf("Create result=%#v err=%v", result, err)
	}
	progress := result.ProgressResult
	if progress.OperationStatus != resource.OperationStatusInProgress || progress.NativeID != readinessFixtureNativeID || progress.RequestID == "" {
		t.Fatalf("known provider acceptance lost to concurrent cancellation: %#v", progress)
	}
	if !strings.Contains(string(progress.ResourceProperties), "custom.googleapis.com/formae/test") {
		t.Fatalf("Create discarded observed provider properties: %s", progress.ResourceProperties)
	}
}

func TestMetricDescriptorCreateMarkerCarriesFixedDeadline(t *testing.T) {
	fixedNow := time.Unix(2_000_000_000, 0)
	provisioner := &metricDescriptorProvisioner{
		BaseResource: &base.BaseResource{},
		now:          func() time.Time { return fixedNow },
		createCall: func(context.Context, *resource.CreateRequest) (*resource.CreateResult, error) {
			return &resource.CreateResult{ProgressResult: &resource.ProgressResult{
				Operation: resource.OperationCreate, OperationStatus: resource.OperationStatusSuccess,
				NativeID: readinessFixtureNativeID,
			}}, nil
		},
	}
	result, err := provisioner.Create(context.Background(), readinessFixtureCreateRequest())
	if err != nil || result == nil || result.ProgressResult == nil {
		t.Fatalf("Create result=%#v err=%v", result, err)
	}
	encoded := strings.TrimPrefix(result.ProgressResult.RequestID, "formae:gcp:metric-descriptor:create-readiness:v1:")
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode readiness marker: %v", err)
	}
	var marker struct {
		NativeID     string `json:"nativeID"`
		DeadlineUnix int64  `json:"deadlineUnix"`
	}
	if err := json.Unmarshal(payload, &marker); err != nil {
		t.Fatalf("decode readiness marker JSON: %v", err)
	}
	if marker.NativeID != readinessFixtureNativeID || marker.DeadlineUnix != fixedNow.Add(10*time.Minute).Unix() {
		t.Fatalf("readiness marker=%#v, want original identity and fixed ten-minute deadline", marker)
	}
}

func TestMetricDescriptorCreateRejectsSuccessWithoutUsableIdentity(t *testing.T) {
	provisioner := &metricDescriptorProvisioner{
		BaseResource: &base.BaseResource{},
		createCall: func(context.Context, *resource.CreateRequest) (*resource.CreateResult, error) {
			return &resource.CreateResult{ProgressResult: &resource.ProgressResult{
				Operation: resource.OperationCreate, OperationStatus: resource.OperationStatusSuccess,
			}}, nil
		},
	}
	result, err := provisioner.Create(context.Background(), readinessFixtureCreateRequest())
	if err != nil || result == nil || result.ProgressResult == nil {
		t.Fatalf("Create result=%#v err=%v", result, err)
	}
	progress := result.ProgressResult
	if progress.OperationStatus != resource.OperationStatusFailure || progress.ErrorCode != resource.OperationErrorCodeGeneralServiceException ||
		progress.NativeID != "" || progress.RequestID != "" {
		t.Fatalf("success without identity must not fabricate readiness proof: %#v", progress)
	}
}

func TestMetricDescriptorReadinessNonterminalErrorsStayPending(t *testing.T) {
	codes := []resource.OperationErrorCode{
		resource.OperationErrorCodeNotUpdatable,
		resource.OperationErrorCodeUnauthorizedTaggingOperation,
		resource.OperationErrorCodeInvalidCredentials,
		resource.OperationErrorCodeAlreadyExists,
		resource.OperationErrorCodeNotFound,
		resource.OperationErrorCodeResourceConflict,
		resource.OperationErrorCodeThrottling,
		resource.OperationErrorCodeServiceLimitExceeded,
		resource.OperationErrorCodeNotStabilized,
		resource.OperationErrorCodeGeneralServiceException,
		resource.OperationErrorCodeServiceInternalError,
		resource.OperationErrorCodeServiceTimeout,
		resource.OperationErrorCodeNetworkFailure,
		resource.OperationErrorCodeInternalFailure,
		resource.OperationErrorCodeDependencyFailure,
		resource.OperationErrorCodeUnforeseenError,
		resource.OperationErrorCodePluginNotFound,
	}
	request := readinessFixtureRequest(t, readinessFixtureNativeID, time.Now().Add(5*time.Minute))
	for _, code := range codes {
		t.Run(string(code), func(t *testing.T) {
			provisioner := registry.Get(MetricDescriptorResourceType, resource.OperationCheckStatus, &config.Config{Project: "test-project"}).(*metricDescriptorProvisioner)
			provisioner.readCall = func(context.Context, *resource.ReadRequest) (*resource.ReadResult, error) {
				return &resource.ReadResult{ErrorCode: code}, nil
			}
			result, err := provisioner.Status(context.Background(), request)
			if err != nil || result == nil || result.ProgressResult == nil {
				t.Fatalf("Status result=%#v err=%v", result, err)
			}
			progress := result.ProgressResult
			if progress.Operation != resource.OperationCreate || progress.OperationStatus != resource.OperationStatusInProgress || progress.ErrorCode != "" {
				t.Fatalf("read error %s became a replayable or terminal result: %#v", code, progress)
			}
		})
	}

	provisioner := registry.Get(MetricDescriptorResourceType, resource.OperationCheckStatus, &config.Config{Project: "test-project"}).(*metricDescriptorProvisioner)
	provisioner.readCall = func(context.Context, *resource.ReadRequest) (*resource.ReadResult, error) {
		return nil, errors.New("connection dropped")
	}
	result, err := provisioner.Status(context.Background(), request)
	if err != nil || result == nil || result.ProgressResult == nil ||
		result.ProgressResult.Operation != resource.OperationCreate ||
		result.ProgressResult.OperationStatus != resource.OperationStatusInProgress || result.ProgressResult.ErrorCode != "" {
		t.Fatalf("Go read error must remain pending: result=%#v err=%v", result, err)
	}
}

func TestMetricDescriptorReadinessExpiresAtOriginalDeadlineAfterRecreation(t *testing.T) {
	deadline := time.Unix(2_000_000_600, 0)
	request := readinessFixtureRequest(t, readinessFixtureNativeID, deadline)
	var reads atomic.Int32
	read := func(context.Context, *resource.ReadRequest) (*resource.ReadResult, error) {
		reads.Add(1)
		return &resource.ReadResult{ErrorCode: resource.OperationErrorCodeNotFound}, nil
	}

	before := registry.Get(MetricDescriptorResourceType, resource.OperationCheckStatus, &config.Config{}).(*metricDescriptorProvisioner)
	before.now = func() time.Time { return deadline.Add(-time.Nanosecond) }
	before.readCall = read
	result, err := before.Status(context.Background(), request)
	if err != nil || result == nil || result.ProgressResult == nil ||
		result.ProgressResult.OperationStatus != resource.OperationStatusInProgress {
		t.Fatalf("Status before deadline result=%#v err=%v", result, err)
	}

	after := registry.Get(MetricDescriptorResourceType, resource.OperationCheckStatus, &config.Config{}).(*metricDescriptorProvisioner)
	after.now = func() time.Time { return deadline }
	after.readCall = read
	result, err = after.Status(context.Background(), request)
	if err != nil || result == nil || result.ProgressResult == nil {
		t.Fatalf("Status at deadline result=%#v err=%v", result, err)
	}
	progress := result.ProgressResult
	if progress.Operation != resource.OperationCreate || progress.OperationStatus != resource.OperationStatusFailure ||
		progress.ErrorCode != resource.OperationErrorCodeGeneralServiceException || resource.IsRecoverable(progress.ErrorCode) {
		t.Fatalf("Status at original deadline must terminate without POST retry: %#v", progress)
	}
	if progress.RequestID != request.RequestID || progress.NativeID != request.NativeID {
		t.Fatalf("expiry lost original operation identity: %#v", progress)
	}
	if reads.Load() != 1 {
		t.Fatalf("expired recreated provisioner performed a provider read: total reads=%d", reads.Load())
	}
}

func TestMetricDescriptorLegacyStatusReadAndDeleteRemainSynchronous(t *testing.T) {
	var gets, deletes atomic.Int32
	server := monitoringAuthenticatedServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			gets.Add(1)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"message":"gone"}}`))
		case http.MethodDelete:
			deletes.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request method %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	statusProvisioner := readinessFixtureProvisioner(t, server.URL, resource.OperationCheckStatus)
	statusResult, err := statusProvisioner.Status(context.Background(), &resource.StatusRequest{
		NativeID: readinessFixtureNativeID, RequestID: "legacy-operation", ResourceType: MetricDescriptorResourceType,
		TargetConfig: []byte(`{"Project":"test-project"}`),
	})
	if err != nil || statusResult == nil || statusResult.ProgressResult == nil {
		t.Fatalf("legacy Status result=%#v err=%v", statusResult, err)
	}
	legacy := statusResult.ProgressResult
	if legacy.Operation != resource.OperationCheckStatus || legacy.OperationStatus != resource.OperationStatusSuccess ||
		legacy.NativeID != readinessFixtureNativeID || legacy.RequestID != "legacy-operation" {
		t.Fatalf("legacy Status contract changed: %#v", legacy)
	}
	if gets.Load() != 0 || deletes.Load() != 0 {
		t.Fatal("legacy synchronous Status unexpectedly used the provider API")
	}

	readResult, err := readinessFixtureProvisioner(t, server.URL, resource.OperationRead).Read(context.Background(), &resource.ReadRequest{
		NativeID: readinessFixtureNativeID, ResourceType: MetricDescriptorResourceType,
		TargetConfig: []byte(`{"Project":"test-project"}`),
	})
	if err != nil || readResult == nil || readResult.ErrorCode != resource.OperationErrorCodeNotFound {
		t.Fatalf("ordinary Read no longer reports genuine absence: result=%#v err=%v", readResult, err)
	}

	deleteResult, err := readinessFixtureProvisioner(t, server.URL, resource.OperationDelete).Delete(context.Background(), &resource.DeleteRequest{
		NativeID: readinessFixtureNativeID, ResourceType: MetricDescriptorResourceType,
		TargetConfig: []byte(`{"Project":"test-project"}`),
	})
	if err != nil || deleteResult == nil || deleteResult.ProgressResult == nil ||
		deleteResult.ProgressResult.Operation != resource.OperationDelete || deleteResult.ProgressResult.OperationStatus != resource.OperationStatusSuccess {
		t.Fatalf("ordinary Delete contract changed: result=%#v err=%v", deleteResult, err)
	}
	if gets.Load() != 1 || deletes.Load() != 1 {
		t.Fatalf("ordinary operations made gets=%d deletes=%d, want one each", gets.Load(), deletes.Load())
	}
}

func readinessFixtureStatus(t *testing.T, baseURL string) prov.Provisioner {
	return readinessFixtureProvisioner(t, baseURL, resource.OperationCheckStatus)
}

func readinessFixtureProvisioner(t *testing.T, baseURL string, operation resource.Operation) prov.Provisioner {
	t.Helper()
	// Both registry factories copy their endpoint during construction. Keep the
	// real registered implementation, including the old generic Status path,
	// while directing any request to the local fixture. These tests are serial.
	definition := monitoringRegistry.Definitions[MetricDescriptorResourceType]
	originalDefinitionURL, originalMonitoringURL := definition.APIConfig.BaseURL, MonitoringAPI.BaseURL
	defer func() {
		definition.APIConfig.BaseURL = originalDefinitionURL
		MonitoringAPI.BaseURL = originalMonitoringURL
	}()
	definition.APIConfig.BaseURL = baseURL + "/v3"
	MonitoringAPI.BaseURL = baseURL + "/v3"
	return registry.Get(MetricDescriptorResourceType, operation, &config.Config{Project: "test-project"})
}

// Fixed wire contract for the proposed durable marker; the test intentionally
// does not call production encoding helpers to generate its oracle.
func readinessFixtureRequest(t *testing.T, nativeID string, deadline time.Time) *resource.StatusRequest {
	t.Helper()
	return &resource.StatusRequest{
		NativeID:     nativeID,
		RequestID:    readinessFixtureMarker(t, nativeID, deadline),
		ResourceType: MetricDescriptorResourceType,
		TargetConfig: []byte(`{"Project":"test-project"}`),
	}
}

func readinessFixtureMarker(t *testing.T, nativeID string, deadline time.Time) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"nativeID": nativeID, "deadlineUnix": deadline.Unix()})
	if err != nil {
		t.Fatal(err)
	}
	return "formae:gcp:metric-descriptor:create-readiness:v1:" + base64.RawURLEncoding.EncodeToString(payload)
}

func readinessFixtureCreateRequest() *resource.CreateRequest {
	return &resource.CreateRequest{
		ResourceType: MetricDescriptorResourceType,
		TargetConfig: []byte(`{"Project":"test-project"}`),
		Properties:   []byte(`{"name":"custom.googleapis.com/formae/test","metricKind":"GAUGE","valueType":"DOUBLE"}`),
	}
}

// The real transport authenticates only against these local fixture endpoints.
// No ambient credentials or provider API calls are needed.
func monitoringAuthenticatedServer(t *testing.T, api http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/subject":
			_, _ = w.Write([]byte("test-subject-token"))
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`))
		default:
			api(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("GCP_CREDENTIALS_JSON", fmt.Sprintf(
		`{"type":"external_account","audience":"//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/test/providers/test","subject_token_type":"urn:ietf:params:oauth:token-type:jwt","token_url":%q,"credential_source":{"url":%q,"format":{"type":"text"}}}`,
		server.URL+"/token", server.URL+"/subject"))
	return server
}
