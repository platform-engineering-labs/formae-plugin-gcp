// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package container

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseContainerNativeID covers both `locations/` (regional and v1
// zonal) and `zones/` (legacy zonal selfLink) inputs — GCP's Container API
// returns zonal `selfLink` with `zones/<zone>` so the parser must accept
// that round-trip.
func TestParseContainerNativeID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  bool
		expected base.PathContext
	}{
		{
			name:  "regional cluster (locations, path-only)",
			input: "projects/my-proj/locations/us-central1/clusters/my-cluster",
			expected: base.PathContext{
				Project:      "my-proj",
				Location:     "us-central1",
				ResourceType: "clusters",
				ResourceName: "my-cluster",
			},
		},
		{
			name:  "regional cluster (locations, full URL)",
			input: "https://container.googleapis.com/v1/projects/my-proj/locations/us-central1/clusters/my-cluster",
			expected: base.PathContext{
				Project:      "my-proj",
				Location:     "us-central1",
				ResourceType: "clusters",
				ResourceName: "my-cluster",
			},
		},
		{
			name:  "zonal cluster (zones, path-only)",
			input: "projects/my-proj/zones/europe-west4-a/clusters/my-cluster",
			expected: base.PathContext{
				Project:      "my-proj",
				Location:     "europe-west4-a",
				ResourceType: "clusters",
				ResourceName: "my-cluster",
			},
		},
		{
			name:  "zonal cluster (zones, full selfLink shape)",
			input: "https://container.googleapis.com/v1/projects/my-proj/zones/europe-west4-a/clusters/my-cluster",
			expected: base.PathContext{
				Project:      "my-proj",
				Location:     "europe-west4-a",
				ResourceType: "clusters",
				ResourceName: "my-cluster",
			},
		},
		{
			name:  "nested nodePool under regional cluster",
			input: "projects/my-proj/locations/us-central1/clusters/my-cluster/nodePools/pool-1",
			expected: base.PathContext{
				Project:        "my-proj",
				Location:       "us-central1",
				ParentType:     "clusters",
				ParentResource: "my-cluster",
				ResourceType:   "nodePools",
				ResourceName:   "pool-1",
			},
		},
		{
			name:  "nested nodePool under zonal cluster",
			input: "projects/my-proj/zones/europe-west4-a/clusters/my-cluster/nodePools/pool-1",
			expected: base.PathContext{
				Project:        "my-proj",
				Location:       "europe-west4-a",
				ParentType:     "clusters",
				ParentResource: "my-cluster",
				ResourceType:   "nodePools",
				ResourceName:   "pool-1",
			},
		},
		{
			name:    "invalid: neither locations nor zones",
			input:   "projects/my-proj/regions/us-central1/clusters/my-cluster",
			wantErr: true,
		},
		{
			name:    "invalid: missing projects prefix",
			input:   "foo/my-proj/locations/us-central1/clusters/my-cluster",
			wantErr: true,
		},
		{
			name:    "invalid: too few segments",
			input:   "projects/my-proj/locations",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseContainerNativeID(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}
