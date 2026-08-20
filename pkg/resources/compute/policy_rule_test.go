// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"encoding/json"
	"testing"
)

const rulePath = "projects/dev-1/global/firewallPolicies/pol-a/rules/1000"
const secRulePath = "projects/dev-1/global/securityPolicies/pol-a/rules/1000"

// A rule has no identity of its own in the API — it is addressed by
// (policy, priority) — so both must survive the native ID round-trip.
func TestRuleNativeIDRoundTrip(t *testing.T) {
	if got := firewallPolicyRuleKind.nativeID("dev-1", "pol-a", 1000); got != rulePath {
		t.Fatalf("build: %q", got)
	}
	project, policy, priority, err := firewallPolicyRuleKind.parseNativeID(rulePath)
	if err != nil {
		t.Fatal(err)
	}
	if project != "dev-1" || policy != "pol-a" || priority != 1000 {
		t.Errorf("parse: %q %q %d", project, policy, priority)
	}
}

func TestParseRuleNativeIDRejectsOtherShapes(t *testing.T) {
	for _, bad := range []string{
		"projects/dev-1/global/firewallPolicies/pol-a",            // the policy itself
		"projects/dev-1/regions/r/firewallPolicies/pol-a/rules/1", // regional shape
		"projects/dev-1/global/firewallPolicies/pol-a/rules/abc",  // non-numeric priority
		"projects/dev-1/global/networks/n/rules/1000",             // wrong collection
		"",
	} {
		if _, _, _, err := firewallPolicyRuleKind.parseNativeID(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

// The two kinds share one implementation, so each must reject the other's ids —
// otherwise a Cloud Armor rule could be read against a firewall policy URL.
func TestPolicyRuleKindsRejectEachOthersIDs(t *testing.T) {
	if _, _, _, err := firewallPolicyRuleKind.parseNativeID(secRulePath); err == nil {
		t.Error("firewall kind accepted a security policy rule ID")
	}
	if _, _, _, err := securityPolicyRuleKind.parseNativeID(rulePath); err == nil {
		t.Error("security kind accepted a firewall policy rule ID")
	}
	if got := securityPolicyRuleKind.nativeID("dev-1", "pol-a", 1000); got != secRulePath {
		t.Errorf("security nativeID: %q", got)
	}
	project, policy, priority, err := securityPolicyRuleKind.parseNativeID(secRulePath)
	if err != nil {
		t.Fatal(err)
	}
	if project != "dev-1" || policy != "pol-a" || priority != 1000 {
		t.Errorf("parse: %q %q %d", project, policy, priority)
	}
}

// Cloud Armor creates exactly one rule of its own, the catch-all allow at
// 2147483647, and List must not offer it as manageable. A user rule sits below.
func TestSecurityPolicyDefaultRuleIsAboveFloor(t *testing.T) {
	if defaultSecurityRulePriority < securityPolicyRuleKind.priorityFloor {
		t.Error("the default allow rule would be reported as manageable")
	}
	if 1000 >= securityPolicyRuleKind.priorityFloor {
		t.Error("a user rule at 1000 must be below the floor")
	}
}

// Each kind strips only its own policy property.
func TestSecurityPolicyRuleBodyStripsPolicy(t *testing.T) {
	out := securityPolicyRuleKind.body(map[string]interface{}{
		"securityPolicy": "pol-a",
		"priority":       float64(1000),
		"action":         "deny(403)",
	}, true)
	if _, ok := out["securityPolicy"]; ok {
		t.Errorf("policy must not reach the body: %#v", out)
	}
	if out["action"] != "deny(403)" || out["priority"] != float64(1000) {
		t.Errorf("rule fields must survive: %#v", out)
	}
}

// "firewallPolicy" identifies the rule's place in the URL; the API rejects it as
// an unknown body field. "priority" is accepted on addRule, so it stays.
func TestRuleBodyStripsPolicy(t *testing.T) {
	props := map[string]interface{}{
		"firewallPolicy": "pol-a",
		"priority":       float64(1000),
		"action":         "allow",
		"ruleName":       "allow-https",
	}
	out := firewallPolicyRuleKind.body(props, true)
	if _, ok := out["firewallPolicy"]; ok {
		t.Errorf("policy must not reach the body: %#v", out)
	}
	if out["priority"] != float64(1000) {
		t.Errorf("priority should be kept when requested: %#v", out)
	}
	if out["action"] != "allow" || out["ruleName"] != "allow-https" {
		t.Errorf("rule fields must survive: %#v", out)
	}
	// The caller can also drop priority, for verbs that take it as a query param.
	if _, ok := firewallPolicyRuleKind.body(props, false)["priority"]; ok {
		t.Errorf("priority should be dropped when not requested")
	}
	// The input map must not be mutated - the caller reuses it.
	if _, ok := props["firewallPolicy"]; !ok {
		t.Errorf("ruleBody mutated its input")
	}
}

// priority arrives as a JSON number from the API and may be an int from a
// declared forma, so every plausible encoding has to coerce.
func TestPriorityOfAcceptsEveryEncoding(t *testing.T) {
	cases := []map[string]interface{}{
		{"priority": float64(1000)},
		{"priority": 1000},
		{"priority": "1000"},
		{"priority": json.Number("1000")},
	}
	for i, props := range cases {
		got, err := priorityOf(props)
		if err != nil {
			t.Errorf("case %d: %v", i, err)
			continue
		}
		if got != 1000 {
			t.Errorf("case %d: got %d", i, got)
		}
	}
}

func TestPriorityOfRejectsMissingOrJunk(t *testing.T) {
	for _, props := range []map[string]interface{}{
		{},
		{"priority": "not-a-number"},
		{"priority": true},
	} {
		if _, err := priorityOf(props); err == nil {
			t.Errorf("expected error for %#v", props)
		}
	}
}

// GCP's own implied rules sit at 2147483644 and above. List must not report
// them, since they cannot be managed.
func TestImpliedRulePriorityFloorExcludesGCPRules(t *testing.T) {
	for _, implied := range []int{2147483644, 2147483645, 2147483646, 2147483647} {
		if implied < firewallPolicyRuleKind.priorityFloor {
			t.Errorf("implied rule %d would be reported as manageable", implied)
		}
	}
	if 1000 >= firewallPolicyRuleKind.priorityFloor {
		t.Errorf("a user rule at 1000 must be below the floor")
	}
}
