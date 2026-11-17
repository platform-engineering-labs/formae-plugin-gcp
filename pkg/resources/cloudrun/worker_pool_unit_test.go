// © 2026 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit
// +build unit

package cloudrun

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkerPoolBodyBuilder_SendsScalingMode guards that every declared scaling
// field reaches the API, so a MANUAL pool is not silently created as AUTOMATIC.
func TestWorkerPoolBodyBuilder_SendsScalingMode(t *testing.T) {
	body, err := workerPoolBodyBuilder(map[string]interface{}{
		"scaling": map[string]interface{}{
			"scalingMode":         "MANUAL",
			"manualInstanceCount": 2,
			"cpuScaling":          map[string]interface{}{"cpuUtilization": 0.6},
		},
	})
	require.NoError(t, err)

	scaling, ok := body["scaling"].(map[string]interface{})
	require.True(t, ok, "scaling missing from body")
	assert.Equal(t, "MANUAL", scaling["scalingMode"])
	assert.EqualValues(t, 2, scaling["manualInstanceCount"])
	assert.Equal(t, map[string]interface{}{"cpuUtilization": 0.6}, scaling["cpuScaling"])
}

// TestWorkerPoolBodyBuilder_OmitsUnsetScalingMode leaves the mode to GCP when the
// forma does not set it.
func TestWorkerPoolBodyBuilder_OmitsUnsetScalingMode(t *testing.T) {
	body, err := workerPoolBodyBuilder(map[string]interface{}{
		"scaling": map[string]interface{}{"minInstanceCount": 1, "maxInstanceCount": 3},
	})
	require.NoError(t, err)

	scaling := body["scaling"].(map[string]interface{})
	assert.NotContains(t, scaling, "scalingMode")
	assert.NotContains(t, scaling, "manualInstanceCount")
	assert.NotContains(t, scaling, "cpuScaling")
}
