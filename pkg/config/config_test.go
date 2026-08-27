// © 2026 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package config

import (
	"encoding/json"
	"reflect"
	"testing"
)

// A TargetPath carries where a resource lives and nothing that could reach
// Google, so a caller that only needs to build a URL cannot accidentally hold
// something that looks able to authenticate and is not.
func TestTargetPathCarriesOnlyAddressingFields(t *testing.T) {
	raw := json.RawMessage(`{
		"Project": "p", "Region": "r", "Zone": "z", "Location": "l",
		"Auth": {"Type": "Oidc", "WorkloadIdentityProvider": "ignored"}
	}`)

	got := PathFromTargetConfig(raw)
	if got.Project != "p" || got.Region != "r" || got.Zone != "z" || got.Location != "l" {
		t.Fatalf("addressing fields = %+v, want p/r/z/l", got)
	}

	// Exactly the four addressing fields. A credential-bearing one would
	// defeat the reason the type exists.
	if n := reflect.TypeOf(got).NumField(); n != 4 {
		t.Errorf("TargetPath has %d fields, want 4 addressing fields only", n)
	}
}

// A nil target config yields a usable zero value rather than panicking a
// caller that is only after a project id.
func TestTargetPathFromNilConfig(t *testing.T) {
	if got := PathFromTargetConfig(nil); got != (TargetPath{}) {
		t.Errorf("PathFromTargetConfig(nil) = %+v, want the zero value", got)
	}
}
