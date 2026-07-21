// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit
// +build unit

package cloudrun

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vpcAccessTemplate returns a RevisionTemplate props map with Direct VPC egress set,
// as it arrives from the Pkl schema (nested []interface{} / map[string]interface{}).
func vpcAccessTemplate() map[string]interface{} {
	return map[string]interface{}{
		"containers": []interface{}{
			map[string]interface{}{"image": "gcr.io/x/formae"},
		},
		"vpcAccess": map[string]interface{}{
			"egress": "PRIVATE_RANGES_ONLY",
			"networkInterfaces": []interface{}{
				map[string]interface{}{
					"network":    "projects/p/global/networks/formae-vpc",
					"subnetwork": "projects/p/regions/europe-central2/subnetworks/formae-subnet",
				},
			},
		},
	}
}

// TestServiceBodyBuilderVpcAccess asserts the request body nests template.vpcAccess
// with the exact Cloud Run v2 wire shape (egress + networkInterfaces[].network/subnetwork).
func TestServiceBodyBuilderVpcAccess(t *testing.T) {
	props := map[string]interface{}{"template": vpcAccessTemplate()}

	body, err := serviceBodyBuilder(props)
	require.NoError(t, err)

	template, ok := body["template"].(map[string]interface{})
	require.True(t, ok, "body must contain template map")

	vpc, ok := template["vpcAccess"].(map[string]interface{})
	require.True(t, ok, "template must contain vpcAccess map")
	assert.Equal(t, "PRIVATE_RANGES_ONLY", vpc["egress"])

	nis, ok := vpc["networkInterfaces"].([]map[string]interface{})
	require.True(t, ok, "vpcAccess must contain networkInterfaces slice")
	require.Len(t, nis, 1)
	assert.Equal(t, "projects/p/global/networks/formae-vpc", nis[0]["network"])
	assert.Equal(t, "projects/p/regions/europe-central2/subnetworks/formae-subnet", nis[0]["subnetwork"])
}

// TestFilterTemplateKeepsVpcAccess is the round-trip guard: filterTemplate must NOT
// strip vpcAccess now that it is a schema field, otherwise read-back drops it and
// reapply reports spurious drift.
func TestFilterTemplateKeepsVpcAccess(t *testing.T) {
	filtered := filterTemplate(vpcAccessTemplate())

	vpc, ok := filtered["vpcAccess"].(map[string]interface{})
	require.True(t, ok, "filterTemplate must preserve vpcAccess")
	assert.Equal(t, "PRIVATE_RANGES_ONLY", vpc["egress"])
}

// TestGetVolumesSecretItems asserts a secret volume with items (version->path)
// survives the request build — needed to mount a config secret at a known path
// (the whole-config-as-secret Cloud Run path). Cloud Run v2 wire shape:
// volumes[].secret.items[]{version,path}.
func TestGetVolumesSecretItems(t *testing.T) {
	props := map[string]interface{}{
		"volumes": []interface{}{
			map[string]interface{}{
				"name": "config",
				"secret": map[string]interface{}{
					"secret": "formae-agent-config",
					"items": []interface{}{
						map[string]interface{}{"version": "latest", "path": "formae.conf.pkl"},
					},
				},
			},
		},
	}

	vols := getVolumesArray(props)
	require.Len(t, vols, 1)
	sec, ok := vols[0]["secret"].(map[string]interface{})
	require.True(t, ok, "volume must carry secret")
	assert.Equal(t, "formae-agent-config", sec["secret"])

	items, ok := sec["items"].([]map[string]interface{})
	require.True(t, ok, "secret must carry items slice")
	require.Len(t, items, 1)
	assert.Equal(t, "latest", items[0]["version"])
	assert.Equal(t, "formae.conf.pkl", items[0]["path"])
}

// TestServiceResponseRoundTripVpcAccess asserts the full read path returns vpcAccess
// in properties, so desired == actual on reapply.
func TestServiceResponseRoundTripVpcAccess(t *testing.T) {
	apiResponse := map[string]interface{}{
		"name":     "projects/p/locations/europe-central2/services/formae",
		"template": vpcAccessTemplate(),
	}
	ctx := base.TransformContext{Project: "p", Region: "europe-central2"}

	props := serviceResponseTransformer(apiResponse, ctx)

	template, ok := props["template"].(map[string]interface{})
	require.True(t, ok, "props must contain template")
	vpc, ok := template["vpcAccess"].(map[string]interface{})
	require.True(t, ok, "read-back template must contain vpcAccess")
	assert.Equal(t, "PRIVATE_RANGES_ONLY", utils.GetString(vpc, "egress"))
}
