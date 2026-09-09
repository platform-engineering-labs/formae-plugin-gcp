// © 2026 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration

package sql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"google.golang.org/api/googleapi"
)

// TestNestedSQLReadParentSafety checks the provider's actual ambiguous 403
// responses. It makes GET requests only. A random nonexistent project is an
// invalid-project control, which must retain the provider error rather than
// pretending the requested child was confirmed missing.
func TestNestedSQLReadParentSafety(t *testing.T) {
	project := os.Getenv("GCP_PROJECT_ID")
	if project == "" {
		t.Fatal("GCP_PROJECT_ID is required for this live test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cfg := &config.Config{Project: project}
	client, err := transport.NewClient(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	instance := "formae-missing-" + uuid.NewString()[:8]
	for _, scenario := range []struct {
		name, project    string
		parentCode, want resource.OperationErrorCode
	}{
		{"missing instance", project, resource.OperationErrorCodeNotFound, resource.OperationErrorCodeNotFound},
		{"invalid project", "formae-invalid-" + uuid.NewString()[:8], resource.OperationErrorCodeInvalidRequest, resource.OperationErrorCodeInvalidRequest},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			parent := fmt.Sprintf("projects/%s/instances/%s", scenario.project, instance)
			_, parentErr := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: SQLAPI.BaseURL + "/" + parent})
			if parentErr == nil {
				t.Fatal("negative-control parent unexpectedly exists")
			}
			var parentGoogleErr *googleapi.Error
			if !errors.As(parentErr, &parentGoogleErr) {
				t.Fatalf("parent lookup did not reach the provider: %v", parentErr)
			}
			parentCode := transport.ToResourceErrorCode(transport.WrapError(parentErr, "parent read").Code)
			if parentCode != scenario.parentCode {
				t.Fatalf("negative-control parent lookup: error=%v code=%s, want %s", parentErr, parentCode, scenario.parentCode)
			}
			for _, child := range []struct{ resourceType, suffix string }{
				{DatabaseResourceType, "databases/formae_missing"},
				{UserResourceType, "users/formae_missing"},
				{SslCertResourceType, "sslCerts/0000000000000000000000000000000000000000"},
				{BackupRunResourceType, "backupRuns/1"},
			} {
				t.Run(child.resourceType, func(t *testing.T) {
					nativeID := parent + "/" + child.suffix
					_, rawErr := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: SQLAPI.BaseURL + "/" + nativeID})
					var gerr *googleapi.Error
					if !errors.As(rawErr, &gerr) {
						t.Fatalf("expected a provider error, got %v", rawErr)
					}
					t.Logf("child HTTP %d reasons=%v; parent code=%s", gerr.Code, gerr.Errors, parentCode)
					provisioner, err := NewSQLProvisioner(cfg, child.resourceType)
					if err != nil {
						t.Fatal(err)
					}
					target, err := json.Marshal(cfg)
					if err != nil {
						t.Fatal(err)
					}
					result, err := provisioner.Read(ctx, &resource.ReadRequest{ResourceType: child.resourceType, NativeID: nativeID, TargetConfig: target})
					if err != nil {
						t.Fatal(err)
					}
					want := scenario.want
					// BackupRun masks even an invalid project as notAuthorized,
					// unlike the other three child APIs. Preserve that child error;
					// do not replace it with the parent's InvalidRequest or NotFound.
					if scenario.name == "invalid project" && child.resourceType == BackupRunResourceType {
						want = resource.OperationErrorCodeAccessDenied
					}
					if result.ErrorCode != want {
						t.Errorf("Read returned %s, want %s: an unverified parent must not erase managed child state", result.ErrorCode, want)
					}
				})
			}
		})
	}
}
