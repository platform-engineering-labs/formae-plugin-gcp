// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package compute

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// A firewall's network must round-trip as the provider's full self-link, the
// same as Subnetwork, Router and Instance already do (PLA-265).
//
// The Compute API is lenient on input and canonical on output. Its own
// discovery document lists three accepted forms for Firewall.network:
//
//	https://www.googleapis.com/compute/v1/projects/p/global/networks/my-network
//	projects/p/global/networks/my-network
//	global/networks/default
//
// but Network.selfLink is "[Output Only] Server-defined URL for the resource"
// and a read always answers with the full URL whichever form was written -
// verified live 2026-09-08: GCP's own default-allow-icmp reads back as
// https://www.googleapis.com/compute/v1/projects/{p}/global/networks/default.
//
// So the full URL is the only form that survives a round trip. Rewriting it to
// a "projects/..." path on read made an unchanged forma that declared
// `network = net.res.selfLink` - the documented idiom, and what every other
// compute fixture uses - diff a short stored path against a full desired URL
// and plan a replace on every reconcile. That is PLA-251.
func TestFirewallKeepsTheProviderSelfLink(t *testing.T) {
	const selfLink = "https://www.googleapis.com/compute/v1/projects/p/global/networks/my-network"

	out := firewallResponseTransformer(map[string]interface{}{
		"name":    "allow-ssh",
		"network": selfLink,
	}, base.TransformContext{Project: "p"})

	if got := out["network"]; got != selfLink {
		t.Errorf("network = %v, want the provider's self-link %q", got, selfLink)
	}
}

// A short path a user declared themselves is passed through untouched too -
// the transformer must not invent a prefix any more than it strips one.
func TestFirewallDoesNotRewriteAShortNetworkPath(t *testing.T) {
	const shortPath = "projects/p/global/networks/my-network"

	out := firewallResponseTransformer(map[string]interface{}{
		"name":    "allow-ssh",
		"network": shortPath,
	}, base.TransformContext{Project: "p"})

	if got := out["network"]; got != shortPath {
		t.Errorf("network = %v, want %q unchanged", got, shortPath)
	}
}

// The rest of the transformer is unaffected: empty ports arrays are still
// preserved on allowed/denied rules.
func TestFirewallStillNormalizesRules(t *testing.T) {
	out := firewallResponseTransformer(map[string]interface{}{
		"name": "allow-ssh",
		"allowed": []interface{}{
			map[string]interface{}{"IPProtocol": "tcp"},
		},
	}, base.TransformContext{Project: "p"})

	allowed, ok := out["allowed"].([]interface{})
	if !ok || len(allowed) != 1 {
		t.Fatalf("allowed = %#v", out["allowed"])
	}
	rule, _ := allowed[0].(map[string]interface{})
	if _, present := rule["ports"]; !present {
		t.Errorf("ports not defaulted on a rule without them: %#v", rule)
	}
}
