// © 2026 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package gkehub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestGKEHubFeaturePathsAreGlobalAndMembershipPathsKeepTheirLocation(t *testing.T) {
	tests := []struct {
		name string
		ctx  base.PathContext
		want string
	}{
		{
			name: "feature list ignores a regional target default",
			ctx:  base.PathContext{Project: "project-1", Location: "europe-west1", ResourceType: "features", IsList: true},
			want: "/projects/project-1/locations/global/features",
		},
		{
			name: "feature native ID keeps its explicit global location",
			ctx:  base.PathContext{Project: "project-1", Location: "global", ResourceType: "features", ResourceName: "servicemesh"},
			want: "/projects/project-1/locations/global/features/servicemesh",
		},
		{
			name: "feature read does not rewrite an explicit native ID location",
			ctx:  base.PathContext{Project: "project-1", Location: "europe-west1", ResourceType: "features", ResourceName: "servicemesh"},
			want: "/projects/project-1/locations/europe-west1/features/servicemesh",
		},
		{
			name: "membership keeps a regional location",
			ctx:  base.PathContext{Project: "project-1", Location: "europe-west1", ResourceType: "memberships", IsList: true},
			want: "/projects/project-1/locations/europe-west1/memberships",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := gkehubPathBuilder(test.ctx); got != test.want {
				t.Fatalf("gkehubPathBuilder() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestFeatureListUsesGlobalAndReturnsResourceIDs(t *testing.T) {
	var requestedPath string
	server := authenticatedServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"resources":[{"name":"projects/project-1/locations/global/features/servicemesh"}]}`))
	})

	provisioner, err := NewGKEHubProvisioner(&config.Config{}, FeatureResourceType)
	if err != nil {
		t.Fatal(err)
	}
	feature := provisioner.(*GKEHubProvisioner)
	feature.APIConfig.BaseURL = server.URL + "/v1"

	target, _ := json.Marshal(config.Config{Project: "project-1", Location: "europe-west1"})
	result, err := feature.List(context.Background(), &resource.ListRequest{
		ResourceType: FeatureResourceType,
		TargetConfig: target,
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestedPath != "/v1/projects/project-1/locations/global/features" {
		t.Fatalf("Feature.List requested %q", requestedPath)
	}
	wantID := "projects/project-1/locations/global/features/servicemesh"
	if len(result.NativeIDs) != 1 || result.NativeIDs[0] != wantID {
		t.Fatalf("Feature.List native IDs = %v, want [%s]", result.NativeIDs, wantID)
	}
}

func TestMembershipListKeepsRegionalLocationAndReturnsResourceIDs(t *testing.T) {
	var requestedPath string
	server := authenticatedServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"resources":[{"name":"projects/project-1/locations/europe-west1/memberships/cluster-1"}]}`))
	})

	provisioner, err := NewGKEHubProvisioner(&config.Config{}, MembershipResourceType)
	if err != nil {
		t.Fatal(err)
	}
	membership := provisioner.(*GKEHubProvisioner)
	membership.APIConfig.BaseURL = server.URL + "/v1"
	target, _ := json.Marshal(config.Config{Project: "project-1", Location: "europe-west1"})

	result, err := membership.List(context.Background(), &resource.ListRequest{
		ResourceType: MembershipResourceType,
		TargetConfig: target,
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestedPath != "/v1/projects/project-1/locations/europe-west1/memberships" {
		t.Fatalf("Membership.List requested %q", requestedPath)
	}
	wantID := "projects/project-1/locations/europe-west1/memberships/cluster-1"
	if len(result.NativeIDs) != 1 || result.NativeIDs[0] != wantID {
		t.Fatalf("Membership.List native IDs = %v, want [%s]", result.NativeIDs, wantID)
	}
}

func TestFeatureListPropagatesProviderError(t *testing.T) {
	server := authenticatedServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"code":500,"message":"provider failed"}}`, http.StatusInternalServerError)
	})
	provisioner, err := NewGKEHubProvisioner(&config.Config{}, FeatureResourceType)
	if err != nil {
		t.Fatal(err)
	}
	feature := provisioner.(*GKEHubProvisioner)
	feature.APIConfig.BaseURL = server.URL + "/v1"
	target, _ := json.Marshal(config.Config{Project: "project-1", Location: "europe-west1"})

	_, err = feature.List(context.Background(), &resource.ListRequest{ResourceType: FeatureResourceType, TargetConfig: target})
	if err == nil || !strings.Contains(err.Error(), "provider failed") {
		t.Fatalf("Feature.List error = %v, want provider failure", err)
	}
}

func TestFeatureCreateUsesGlobalForRequestAndSynthesizedNativeID(t *testing.T) {
	var method, requestedPath, featureID string
	server := authenticatedServer(t, func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		requestedPath = r.URL.Path
		featureID = r.URL.Query().Get("featureId")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"projects/project-1/locations/global/operations/op-1"}`))
	})
	provisioner, err := NewGKEHubProvisioner(&config.Config{}, FeatureResourceType)
	if err != nil {
		t.Fatal(err)
	}
	feature := provisioner.(*GKEHubProvisioner)
	feature.APIConfig.BaseURL = server.URL + "/v1"
	target, _ := json.Marshal(config.Config{Project: "project-1", Location: "europe-west1"})

	result, err := feature.Create(context.Background(), &resource.CreateRequest{
		ResourceType: FeatureResourceType,
		TargetConfig: target,
		Properties:   []byte(`{"name":"servicemesh"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost || requestedPath != "/v1/projects/project-1/locations/global/features" || featureID != "servicemesh" {
		t.Fatalf("Feature.Create request = %s %s featureId=%q", method, requestedPath, featureID)
	}
	wantID := "projects/project-1/locations/global/features/servicemesh"
	if result.ProgressResult.NativeID != wantID {
		t.Fatalf("Feature.Create native ID = %q, want %q", result.ProgressResult.NativeID, wantID)
	}
}

func TestMembershipCreateKeepsRegionalLocation(t *testing.T) {
	var requestedPath, membershipID string
	server := authenticatedServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		membershipID = r.URL.Query().Get("membershipId")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"projects/project-1/locations/europe-west1/operations/op-1"}`))
	})
	provisioner, err := NewGKEHubProvisioner(&config.Config{}, MembershipResourceType)
	if err != nil {
		t.Fatal(err)
	}
	membership := provisioner.(*GKEHubProvisioner)
	membership.APIConfig.BaseURL = server.URL + "/v1"
	target, _ := json.Marshal(config.Config{Project: "project-1", Location: "europe-west1"})

	result, err := membership.Create(context.Background(), &resource.CreateRequest{
		ResourceType: MembershipResourceType,
		TargetConfig: target,
		Properties:   []byte(`{"name":"cluster-1"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestedPath != "/v1/projects/project-1/locations/europe-west1/memberships" || membershipID != "cluster-1" {
		t.Fatalf("Membership.Create request = %s membershipId=%q", requestedPath, membershipID)
	}
	wantID := "projects/project-1/locations/europe-west1/memberships/cluster-1"
	if result.ProgressResult.NativeID != wantID {
		t.Fatalf("Membership.Create native ID = %q, want %q", result.ProgressResult.NativeID, wantID)
	}
}

func TestFeatureCreateReportsProviderError(t *testing.T) {
	server := authenticatedServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"code":500,"message":"provider failed"}}`, http.StatusInternalServerError)
	})
	provisioner, err := NewGKEHubProvisioner(&config.Config{}, FeatureResourceType)
	if err != nil {
		t.Fatal(err)
	}
	feature := provisioner.(*GKEHubProvisioner)
	feature.APIConfig.BaseURL = server.URL + "/v1"
	target, _ := json.Marshal(config.Config{Project: "project-1", Location: "europe-west1"})

	result, err := feature.Create(context.Background(), &resource.CreateRequest{
		ResourceType: FeatureResourceType,
		TargetConfig: target,
		Properties:   []byte(`{"name":"servicemesh"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProgressResult.OperationStatus != resource.OperationStatusFailure ||
		!strings.Contains(result.ProgressResult.StatusMessage, "provider failed") {
		t.Fatalf("Feature.Create result = %#v, want provider failure", result.ProgressResult)
	}
}

// authenticatedServer gives the real transport an OAuth endpoint and an API
// endpoint. Tests exercise the complete List request without ambient GCP auth.
func authenticatedServer(t *testing.T, api http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/subject" {
			_, _ = w.Write([]byte("test-subject-token"))
			return
		}
		if r.URL.Path == "/token" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`))
			return
		}
		api(w, r)
	}))
	t.Cleanup(server.Close)
	t.Setenv("GCP_CREDENTIALS_JSON", fmt.Sprintf(
		`{"type":"external_account","audience":"//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/test/providers/test","subject_token_type":"urn:ietf:params:oauth:token-type:jwt","token_url":%q,"credential_source":{"url":%q,"format":{"type":"text"}}}`,
		server.URL+"/token", server.URL+"/subject"))
	return server
}
