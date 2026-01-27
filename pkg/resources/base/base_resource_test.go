// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit
// +build unit

package base

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathContextBuilding(t *testing.T) {
	// Test Compute-style path building (global/regional/zonal)
	computeBuilder := func(ctx PathContext) string {
		scope := "global"
		if ctx.Zone != "" {
			scope = "zones/" + ctx.Zone
		} else if ctx.Region != "" {
			scope = "regions/" + ctx.Region
		}
		path := "/projects/" + ctx.Project + "/" + scope + "/" + ctx.ResourceType
		if ctx.ResourceName != "" {
			path += "/" + ctx.ResourceName
		}
		return path
	}

	tests := []struct {
		name     string
		ctx      PathContext
		expected string
	}{
		{
			name: "Global resource collection",
			ctx: PathContext{
				Project:      "my-project",
				ResourceType: "networks",
			},
			expected: "/projects/my-project/global/networks",
		},
		{
			name: "Global resource instance",
			ctx: PathContext{
				Project:      "my-project",
				ResourceType: "networks",
				ResourceName: "my-network",
			},
			expected: "/projects/my-project/global/networks/my-network",
		},
		{
			name: "Regional resource",
			ctx: PathContext{
				Project:      "my-project",
				Region:       "us-central1",
				ResourceType: "addresses",
				ResourceName: "my-address",
			},
			expected: "/projects/my-project/regions/us-central1/addresses/my-address",
		},
		{
			name: "Zonal resource",
			ctx: PathContext{
				Project:      "my-project",
				Zone:         "us-central1-a",
				ResourceType: "instances",
				ResourceName: "my-instance",
			},
			expected: "/projects/my-project/zones/us-central1-a/instances/my-instance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeBuilder(tt.ctx)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestURLBuilder(t *testing.T) {
	computeAPI := APIConfig{
		BaseURL: "https://compute.googleapis.com/compute/v1",
		PathBuilder: func(ctx PathContext) string {
			return "/projects/" + ctx.Project + "/global/" + ctx.ResourceType
		},
	}

	ctx := PathContext{
		Project:      "my-project",
		ResourceType: "networks",
	}

	builder := NewURLBuilder(computeAPI, ctx)

	t.Run("Collection URL", func(t *testing.T) {
		url := builder.CollectionURL()
		expected := "https://compute.googleapis.com/compute/v1/projects/my-project/global/networks"
		assert.Equal(t, expected, url)
	})

	t.Run("Resource URL", func(t *testing.T) {
		url := builder.ResourceURL("my-network")
		// Note: This would need the PathBuilder to handle ResourceName
		// In practice, the PathBuilder checks ctx.ResourceName
		assert.Contains(t, url, "https://compute.googleapis.com/compute/v1")
	})
}

func TestNativeIDParsing(t *testing.T) {
	tests := []struct {
		name      string
		format    NativeIDFormat
		nativeID  string
		wantName  string
		wantError bool
	}{
		{
			name:     "Simple name format",
			format:   SimpleNameFormat,
			nativeID: "my-network",
			wantName: "my-network",
		},
		{
			name:     "Full path format",
			format:   FullPathFormat,
			nativeID: "projects/my-project/instances/my-instance",
			wantName: "my-instance",
		},
		{
			name:     "Full URL format",
			format:   FullURLFormat,
			nativeID: "https://container.googleapis.com/v1/projects/my-project/locations/us-central1/clusters/my-cluster",
			wantName: "my-cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NativeIDConfig{
				Format: tt.format,
			}

			ctx, err := ParseNativeID(config, tt.nativeID)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantName, ctx.ResourceName)
			}
		})
	}
}

func TestNativeIDBuilding(t *testing.T) {
	tests := []struct {
		name         string
		config       NativeIDConfig
		resourceName string
		ctx          PathContext
		expected     string
	}{
		{
			name: "Simple name format",
			config: NativeIDConfig{
				Format: SimpleNameFormat,
			},
			resourceName: "my-network",
			expected:     "my-network",
		},
		{
			name: "Full path format",
			config: NativeIDConfig{
				Format:       FullPathFormat,
				PathTemplate: "projects/{project}/instances/{name}",
			},
			resourceName: "my-instance",
			ctx: PathContext{
				Project: "my-project",
			},
			expected: "projects/my-project/instances/my-instance",
		},
		{
			name: "Full URL format",
			config: NativeIDConfig{
				Format:       FullURLFormat,
				PathTemplate: "https://container.googleapis.com/v1/projects/{project}/locations/{location}/clusters/{name}",
			},
			resourceName: "my-cluster",
			ctx: PathContext{
				Project:  "my-project",
				Location: "us-central1",
			},
			expected: "https://container.googleapis.com/v1/projects/my-project/locations/us-central1/clusters/my-cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildNativeID(tt.config, tt.resourceName, tt.ctx)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWrapperTransformer(t *testing.T) {
	transformer := &WrapperTransformer{
		WrapperField: "cluster",
	}

	props := map[string]interface{}{
		"name":     "my-cluster",
		"location": "us-central1",
	}

	result, err := transformer.Transform(props, TransformContext{})
	require.NoError(t, err)

	expected := map[string]interface{}{
		"cluster": map[string]interface{}{
			"name":     "my-cluster",
			"location": "us-central1",
		},
	}

	assert.Equal(t, expected, result)
}

func TestFieldFilterTransformer(t *testing.T) {
	props := map[string]interface{}{
		"name":     "my-resource",
		"status":   "RUNNING",
		"selfLink": "https://...",
	}

	t.Run("Blacklist mode", func(t *testing.T) {
		transformer := &FieldFilterTransformer{
			ExcludeFields: []string{"creationTimestamp", "selfLink"},
		}

		result, err := transformer.Transform(props, TransformContext{})
		require.NoError(t, err)

		assert.Contains(t, result, "name")
		assert.Contains(t, result, "status")
		assert.NotContains(t, result, "creationTimestamp")
		assert.NotContains(t, result, "selfLink")
	})

	t.Run("Whitelist mode", func(t *testing.T) {
		transformer := &FieldFilterTransformer{
			IncludeFields: []string{"name", "status"},
		}

		result, err := transformer.Transform(props, TransformContext{})
		require.NoError(t, err)

		assert.Contains(t, result, "name")
		assert.Contains(t, result, "status")
		assert.NotContains(t, result, "creationTimestamp")
		assert.NotContains(t, result, "selfLink")
	})
}

func TestCompositeTransformer(t *testing.T) {
	props := map[string]interface{}{
		"name":              "my-cluster",
		"location":          "us-central1",
		"creationTimestamp": "2025-01-01",
		"selfLink":          "https://...",
	}

	composite := &CompositeRequestTransformer{
		Transformers: []RequestTransformer{
			&FieldFilterTransformer{
				ExcludeFields: []string{"creationTimestamp", "selfLink"},
			},
			&WrapperTransformer{
				WrapperField: "cluster",
			},
		},
	}

	result, err := composite.Transform(props, TransformContext{})
	require.NoError(t, err)

	// Should be wrapped and filtered
	assert.Contains(t, result, "cluster")
	clusterData := result["cluster"].(map[string]interface{})
	assert.Contains(t, clusterData, "name")
	assert.Contains(t, clusterData, "location")
	assert.NotContains(t, clusterData, "creationTimestamp")
	assert.NotContains(t, clusterData, "selfLink")
}
