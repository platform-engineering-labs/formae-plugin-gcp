// © 2026 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/container"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/gkehub"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"google.golang.org/api/googleapi"
)

// TestGCPDiscoveryProviderPaths is a read-only provider integration regression.
// It exercises the real List provisioners with the same regional and wildcard
// target defaults that discovery supplies, then compares them with direct API
// responses. Full CRUD conformance remains the responsibility of testdata
// fixtures; this probe creates and deletes no cloud resources.
func TestGCPDiscoveryProviderPaths(t *testing.T) {
	project := requireEnv(t, "GCP_PROJECT_ID")
	region := requireEnv(t, "GCP_REGION")
	zone := requireEnv(t, "GCP_ZONE")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cfg := &config.Config{Project: project}
	client, err := transport.NewClient(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("feature list uses global and matches direct list", func(t *testing.T) {
		provisioner := registry.Get(gkehub.FeatureResourceType, resource.OperationList, cfg)
		got, err := provisioner.List(ctx, &resource.ListRequest{
			ResourceType: gkehub.FeatureResourceType,
			TargetConfig: targetConfig(t, project, region),
		})
		if err != nil {
			t.Fatal(err)
		}

		direct := directListNames(t, ctx, client,
			gkehub.GKEHubAPI.BaseURL+fmt.Sprintf("/projects/%s/locations/global/features", project), "resources")
		assertSameIDs(t, got.NativeIDs, direct)
		t.Logf("matched %d Feature IDs", len(direct))

		_, regionalErr := client.SendRequest(ctx, transport.RequestOptions{
			Method: "GET",
			URL:    gkehub.GKEHubAPI.BaseURL + fmt.Sprintf("/projects/%s/locations/%s/features", project, region),
		})
		assertProviderStatus(t, regionalErr, 400)
	})

	t.Run("membership list keeps regional location and matches direct list", func(t *testing.T) {
		provisioner := registry.Get(gkehub.MembershipResourceType, resource.OperationList, cfg)
		got, err := provisioner.List(ctx, &resource.ListRequest{
			ResourceType: gkehub.MembershipResourceType,
			TargetConfig: targetConfig(t, project, region),
		})
		if err != nil {
			t.Fatal(err)
		}
		direct := directListNames(t, ctx, client,
			gkehub.GKEHubAPI.BaseURL+fmt.Sprintf("/projects/%s/locations/%s/memberships", project, region), "resources")
		assertSameIDs(t, got.NativeIDs, direct)
		t.Logf("matched %d Membership IDs", len(direct))
	})

	t.Run("node pool list uses the parent location", func(t *testing.T) {
		clusterName := "formae-test-gke-missing-" + uuid.NewString()[:8]
		clusterLocation := zone
		target, err := json.Marshal(config.Config{Project: project, Location: "-"})
		if err != nil {
			t.Fatal(err)
		}
		provisioner := registry.Get(container.NodePoolResourceType, resource.OperationList, cfg)
		_, listErr := provisioner.List(ctx, &resource.ListRequest{
			ResourceType: container.NodePoolResourceType,
			TargetConfig: target,
			AdditionalProperties: map[string]string{
				"cluster": clusterName, "location": clusterLocation,
			},
		})
		assertProviderStatus(t, listErr, 404)
	})
}

func directListNames(t *testing.T, ctx context.Context, client *transport.Client, url, key string) []string {
	t.Helper()
	response, err := client.SendRequest(ctx, transport.RequestOptions{Method: "GET", URL: url})
	if err != nil {
		t.Fatal(err)
	}
	items, _ := response.Body[key].([]interface{})
	ids := make([]string, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]interface{})
		if name, _ := item["name"].(string); name != "" {
			ids = append(ids, name)
		}
	}
	return ids
}

func assertSameIDs(t *testing.T, got, want []string) {
	t.Helper()
	got = append([]string(nil), got...)
	want = append([]string(nil), want...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("provisioner native IDs = %v, direct API names = %v", got, want)
	}
}

func assertProviderStatus(t *testing.T, err error, status int) {
	t.Helper()
	var googleErr *googleapi.Error
	if !errors.As(err, &googleErr) || googleErr.Code != status {
		t.Fatalf("provider error = %v, want HTTP %d", err, status)
	}
}

func targetConfig(t *testing.T, project, location string) []byte {
	t.Helper()
	target, err := json.Marshal(config.Config{Project: project, Location: location})
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func requireEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is required for this live test", name)
	}
	return value
}
