// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import (
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// SELF_MANAGED certs must nest certificate + privateKey under `selfManaged`;
// GCP rejects them at the top level ("Self-managed certificate details must be
// specified if type = SELF_MANAGED").
func TestSslCertRequestTransformerSelfManaged(t *testing.T) {
	out, err := sslCertificateRequestTransformer(map[string]interface{}{
		"name":        "c",
		"type":        "SELF_MANAGED",
		"certificate": "-----BEGIN CERTIFICATE-----\nX\n-----END CERTIFICATE-----",
		"privateKey":  "-----BEGIN PRIVATE KEY-----\nY\n-----END PRIVATE KEY-----",
	}, base.TransformContext{})
	if err != nil {
		t.Fatal(err)
	}
	sm, ok := out["selfManaged"].(map[string]interface{})
	if !ok {
		t.Fatalf("selfManaged not nested: %#v", out)
	}
	if sm["certificate"] == "" || sm["privateKey"] == "" {
		t.Errorf("selfManaged missing cert/key: %#v", sm)
	}
	if _, ok := out["certificate"]; ok {
		t.Errorf("certificate must not remain top-level: %#v", out)
	}
	if _, ok := out["privateKey"]; ok {
		t.Errorf("privateKey must not remain top-level: %#v", out)
	}
	if out["type"] != "SELF_MANAGED" || out["name"] != "c" {
		t.Errorf("other fields must survive: %#v", out)
	}
}

// MANAGED nesting (managedDomains -> managed.domains) must still work.
func TestSslCertRequestTransformerManaged(t *testing.T) {
	out, err := sslCertificateRequestTransformer(map[string]interface{}{
		"name":           "c",
		"type":           "MANAGED",
		"managedDomains": []interface{}{"formae.example.com"},
	}, base.TransformContext{})
	if err != nil {
		t.Fatal(err)
	}
	m, ok := out["managed"].(map[string]interface{})
	if !ok {
		t.Fatalf("managed not nested: %#v", out)
	}
	if !reflect.DeepEqual(m["domains"], []interface{}{"formae.example.com"}) {
		t.Errorf("managed.domains wrong: %#v", m)
	}
	if _, ok := out["managedDomains"]; ok {
		t.Errorf("managedDomains must not remain: %#v", out)
	}
}

// Read lifts selfManaged.certificate back to the flat field and drops the
// wrapper so the state round-trips (privateKey is write-only, never returned).
func TestSslCertResponseTransformerSelfManaged(t *testing.T) {
	out := sslCertificateResponseTransformer(map[string]interface{}{
		"name":        "c",
		"type":        "SELF_MANAGED",
		"selfManaged": map[string]interface{}{"certificate": "PEM"},
	}, base.TransformContext{})
	if out["certificate"] != "PEM" {
		t.Errorf("certificate not lifted: %#v", out)
	}
	if _, ok := out["selfManaged"]; ok {
		t.Errorf("selfManaged wrapper must be dropped: %#v", out)
	}
}

// Read lifts managed.domains back to managedDomains and drops the wrapper.
func TestSslCertResponseTransformerManaged(t *testing.T) {
	out := sslCertificateResponseTransformer(map[string]interface{}{
		"name":    "c",
		"type":    "MANAGED",
		"managed": map[string]interface{}{"domains": []interface{}{"formae.example.com"}},
	}, base.TransformContext{})
	if !reflect.DeepEqual(out["managedDomains"], []interface{}{"formae.example.com"}) {
		t.Errorf("managedDomains not lifted: %#v", out)
	}
	if _, ok := out["managed"]; ok {
		t.Errorf("managed wrapper must be dropped: %#v", out)
	}
}
