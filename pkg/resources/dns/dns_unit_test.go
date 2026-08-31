// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package dns

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

func TestPathBuilderTopLevelAndNested(t *testing.T) {
	cases := []struct {
		name string
		ctx  base.PathContext
		want string
	}{
		{
			name: "policy collection",
			ctx:  base.PathContext{Project: "p", ResourceType: "policies"},
			want: "/projects/p/policies",
		},
		{
			name: "policy resource",
			ctx:  base.PathContext{Project: "p", ResourceType: "policies", ResourceName: "pol"},
			want: "/projects/p/policies/pol",
		},
		{
			name: "rule under its response policy",
			ctx: base.PathContext{Project: "p", ResourceType: "rules", ResourceName: "r1",
				ParentType: "responsePolicies", ParentResource: "rp"},
			want: "/projects/p/responsePolicies/rp/rules/r1",
		},
	}
	for _, tc := range cases {
		if got := dnsPathBuilder(tc.ctx); got != tc.want {
			t.Errorf("%s: path = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A rule's parent lives only in its native ID; losing it on parse would address
// the project-level collection and 404 on every read.
func TestParseNativeIDRestoresTheParent(t *testing.T) {
	ctx, err := parseDNSNativeID("projects/p/responsePolicies/rp/rules/r1")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ctx.ParentType != "responsePolicies" || ctx.ParentResource != "rp" {
		t.Errorf("parent = %s/%s", ctx.ParentType, ctx.ParentResource)
	}
	if ctx.ResourceType != "rules" || ctx.ResourceName != "r1" {
		t.Errorf("resource = %s/%s", ctx.ResourceType, ctx.ResourceName)
	}

	top, err := parseDNSNativeID("projects/p/policies/pol")
	if err != nil {
		t.Fatalf("parse top level: %v", err)
	}
	if top.ParentType != "" || top.ResourceName != "pol" {
		t.Errorf("top-level ctx = %+v", top)
	}

	for _, bad := range []string{"", "projects/p", "zones/p/policies/x", "projects/p/a/b/c/d/e/f"} {
		if _, err := parseDNSNativeID(bad); err == nil {
			t.Errorf("expected an error for %q", bad)
		}
	}
}

// Cloud DNS does not agree with itself about what the identifier is called: a
// policy uses "name", a response policy "responsePolicyName", a rule
// "ruleName". A List item is the only place the id appears, so all three shapes
// have to yield a native ID.
func TestNativeIDAcceptsEveryIdentifierSpelling(t *testing.T) {
	cases := map[string]struct {
		item map[string]interface{}
		ctx  base.PathContext
		want string
	}{
		"policy": {
			map[string]interface{}{"name": "pol"},
			base.PathContext{Project: "p", ResourceType: "policies"},
			"projects/p/policies/pol",
		},
		"responsePolicy": {
			map[string]interface{}{"responsePolicyName": "rp"},
			base.PathContext{Project: "p", ResourceType: "responsePolicies"},
			"projects/p/responsePolicies/rp",
		},
		"rule": {
			map[string]interface{}{"ruleName": "r1"},
			base.PathContext{Project: "p", ResourceType: "rules",
				ParentType: "responsePolicies", ParentResource: "rp"},
			"projects/p/responsePolicies/rp/rules/r1",
		},
	}
	for name, tc := range cases {
		if got := extractDNSNativeID(tc.item, tc.ctx); got != tc.want {
			t.Errorf("%s: native id = %q, want %q", name, got, tc.want)
		}
	}
}

// A forma declares "name" for every resource in this plugin. The API wants
// "responsePolicyName" and "ruleName", so the translation happens at the
// boundary and round-trips.
func TestIdentifierRenamesRoundTrip(t *testing.T) {
	req, err := responsePolicyRequestTransformer(
		map[string]interface{}{"name": "rp", "description": "d"}, base.TransformContext{})
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	if req["responsePolicyName"] != "rp" || req["description"] != "d" {
		t.Errorf("request = %+v", req)
	}
	if _, present := req["name"]; present {
		t.Errorf("name must not survive into the body: %+v", req)
	}
	back := responsePolicyResponseTransformer(req, base.TransformContext{})
	if back["name"] != "rp" {
		t.Errorf("response = %+v", back)
	}

	ruleReq, err := responsePolicyRuleRequestTransformer(
		map[string]interface{}{"name": "r1", "responsePolicy": "rp", "dnsName": "x.example.com."},
		base.TransformContext{})
	if err != nil {
		t.Fatalf("rule transform: %v", err)
	}
	if ruleReq["ruleName"] != "r1" {
		t.Errorf("rule request = %+v", ruleReq)
	}
	// responsePolicy addresses the rule; the API rejects it as a body field.
	if _, present := ruleReq["responsePolicy"]; present {
		t.Errorf("responsePolicy must not reach the body: %+v", ruleReq)
	}
	if back := responsePolicyRuleResponseTransformer(ruleReq, base.TransformContext{}); back["name"] != "r1" {
		t.Errorf("rule response = %+v", back)
	}
}

func TestAllDNSTypesAreRegistered(t *testing.T) {
	for _, rt := range []string{
		ManagedZoneResourceType, PolicyResourceType,
		ResponsePolicyResourceType, ResponsePolicyRuleResourceType,
	} {
		for _, op := range []resource.Operation{
			resource.OperationCreate, resource.OperationRead,
			resource.OperationDelete, resource.OperationList,
		} {
			if !registry.HasProvisioner(rt, op) {
				t.Errorf("%s %v not registered", rt, op)
			}
		}
	}
}

// Rules must keep the parent-walking List: discovery names no policy and Cloud
// DNS has no wildcard for that segment.
func TestRuleListWalksTheResponsePolicies(t *testing.T) {
	p := registry.Get(ResponsePolicyRuleResourceType, resource.OperationList, nil)
	if _, ok := p.(*responsePolicyRuleListProvisioner); !ok {
		t.Errorf("rule List is %T, want *responsePolicyRuleListProvisioner", p)
	}
}
