// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package dns

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// Cloud DNS stamps "kind" on nested objects as well as the policy itself. Left
// in, each one reads as a property the forma never declared: the conformance
// comparison failed on networks[0].kind after create, sync and update.
func TestPolicyResponseStripsNestedKind(t *testing.T) {
	out := policyResponseTransformer(map[string]interface{}{
		"kind": "dns#policy",
		"name": "p1",
		"networks": []interface{}{
			map[string]interface{}{"kind": "dns#policyNetwork", "networkUrl": "https://example/net"},
		},
		"alternativeNameServerConfig": map[string]interface{}{
			"kind": "dns#policyAlternativeNameServerConfig",
			"targetNameServers": []interface{}{
				map[string]interface{}{"kind": "dns#policyAlternativeNameServerConfigTargetNameServer", "ipv4Address": "10.0.0.1"},
			},
		},
	}, base.TransformContext{Project: "p"})

	if _, ok := out["kind"]; ok {
		t.Error("top-level kind survived")
	}
	nets, _ := out["networks"].([]interface{})
	n0, _ := nets[0].(map[string]interface{})
	if _, ok := n0["kind"]; ok {
		t.Error("networks[0].kind survived")
	}
	if n0["networkUrl"] != "https://example/net" {
		t.Errorf("networkUrl was lost: %v", n0)
	}
	cfg, _ := out["alternativeNameServerConfig"].(map[string]interface{})
	if _, ok := cfg["kind"]; ok {
		t.Error("alternativeNameServerConfig.kind survived")
	}
	tns, _ := cfg["targetNameServers"].([]interface{})
	t0, _ := tns[0].(map[string]interface{})
	if _, ok := t0["kind"]; ok {
		t.Error("targetNameServers[0].kind survived")
	}
	if t0["ipv4Address"] != "10.0.0.1" {
		t.Errorf("ipv4Address was lost: %v", t0)
	}
	if out["project"] != "p" {
		t.Errorf("project = %v, want p", out["project"])
	}
}

// A response policy's id field is responsePolicyName, not name. A listed item
// carries no path context, so without that fallback every response policy would
// list with an empty native ID and never be discovered.
func TestNativeIDHandlesResponsePolicyName(t *testing.T) {
	got := extractDNSNativeID(
		map[string]interface{}{"responsePolicyName": "rp1"},
		base.PathContext{Project: "p", ResourceType: "responsePolicies"},
	)
	if got != "projects/p/responsePolicies/rp1" {
		t.Errorf("native ID = %q", got)
	}
	// A policy still uses name.
	got = extractDNSNativeID(
		map[string]interface{}{"name": "p1"},
		base.PathContext{Project: "p", ResourceType: "policies"},
	)
	if got != "projects/p/policies/p1" {
		t.Errorf("native ID = %q", got)
	}
}
