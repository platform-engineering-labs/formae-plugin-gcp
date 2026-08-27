// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package sql

import (
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// The fingerprint arrives nested under clientCert.certInfo on insert and at the
// top level on a get or a list item. Reading only one shape would leave creates
// and reads disagreeing about the resource's identity.
func TestSslCertFingerprintFromBothShapes(t *testing.T) {
	insert := map[string]interface{}{
		"clientCert": map[string]interface{}{
			"certInfo":       map[string]interface{}{"sha1Fingerprint": "abc123", "commonName": "client"},
			"certPrivateKey": "-----BEGIN PRIVATE KEY-----",
		},
	}
	if got := sslCertFingerprint(insert); got != "abc123" {
		t.Errorf("insert fingerprint = %q", got)
	}
	get := map[string]interface{}{"sha1Fingerprint": "def456"}
	if got := sslCertFingerprint(get); got != "def456" {
		t.Errorf("get fingerprint = %q", got)
	}
	if got := sslCertFingerprint(map[string]interface{}{}); got != "" {
		t.Errorf("expected no fingerprint, got %q", got)
	}
}

func TestSslCertNativeIDUsesTheFingerprint(t *testing.T) {
	ctx := base.PathContext{Project: "proj", ParentType: "instances", ParentResource: "inst"}
	got := extractSslCertNativeID(map[string]interface{}{"sha1Fingerprint": "abc123"}, ctx)
	if got != "projects/proj/instances/inst/sslCerts/abc123" {
		t.Errorf("native id = %q", got)
	}
	// Without an instance there is nothing to address the certificate under.
	if got := extractSslCertNativeID(map[string]interface{}{"sha1Fingerprint": "abc"}, base.PathContext{}); got != "" {
		t.Errorf("expected empty native id, got %q", got)
	}
}

// The private key is returned exactly once. Persisting it would put a private
// key in stored state and guarantee drift on every later read, since the API
// never returns it again.
func TestSslCertResponseFlattensAndDropsThePrivateKey(t *testing.T) {
	out := sslCertResponseTransformer(map[string]interface{}{
		"clientCert": map[string]interface{}{
			"certInfo": map[string]interface{}{
				"sha1Fingerprint": "abc123",
				"commonName":      "client",
				"instance":        "inst",
				"kind":            "sql#sslCert",
			},
			"certPrivateKey": "-----BEGIN PRIVATE KEY-----",
		},
		"operation": map[string]interface{}{"name": "op-1"},
	}, base.TransformContext{})

	if out["commonName"] != "client" || out["sha1Fingerprint"] != "abc123" {
		t.Errorf("cert = %+v", out)
	}
	for _, k := range []string{"certPrivateKey", "clientCert", "operation", "kind"} {
		if _, present := out[k]; present {
			t.Errorf("%s must not survive into stored state: %+v", k, out)
		}
	}
}

// A get answers with the certificate directly rather than wrapped, so the same
// transformer has to leave an already-flat response alone.
func TestSslCertResponseLeavesAFlatReadAlone(t *testing.T) {
	out := sslCertResponseTransformer(map[string]interface{}{
		"sha1Fingerprint": "abc123", "commonName": "client",
	}, base.TransformContext{})
	if out["commonName"] != "client" || out["sha1Fingerprint"] != "abc123" {
		t.Errorf("cert = %+v", out)
	}
}

// A backup run's id arrives as backupContext.backupId on the create Operation
// and as "id" on a get or list item. Both are strings - Google renders int64 as
// a string - so nothing here may parse a number.
func TestBackupRunIDFromBothShapes(t *testing.T) {
	operation := map[string]interface{}{
		"name":          "op-1",
		"backupContext": map[string]interface{}{"backupId": "1234567890"},
	}
	if got := backupRunID(operation); got != "1234567890" {
		t.Errorf("operation id = %q", got)
	}
	if got := backupRunID(map[string]interface{}{"id": "987"}); got != "987" {
		t.Errorf("get id = %q", got)
	}
	if got := backupRunID(map[string]interface{}{}); got != "" {
		t.Errorf("expected no id, got %q", got)
	}
}

func TestBackupRunNativeIDUsesTheServerAssignedID(t *testing.T) {
	ctx := base.PathContext{Project: "proj", ParentType: "instances", ParentResource: "inst"}
	got := extractBackupRunNativeID(map[string]interface{}{
		"backupContext": map[string]interface{}{"backupId": "1234567890"},
	}, ctx)
	if got != "projects/proj/instances/inst/backupRuns/1234567890" {
		t.Errorf("native id = %q", got)
	}
}

func TestBackupRunResponseDropsEchoedFields(t *testing.T) {
	out := backupRunResponseTransformer(map[string]interface{}{
		"id": "1", "instance": "inst", "status": "SUCCESSFUL",
		"kind": "sql#backupRun", "selfLink": "https://...", "project": "proj",
	}, base.TransformContext{})
	if out["id"] != "1" || out["instance"] != "inst" || out["status"] != "SUCCESSFUL" {
		t.Errorf("backup run = %+v", out)
	}
	for _, k := range []string{"kind", "selfLink", "project"} {
		if _, present := out[k]; present {
			t.Errorf("%s should have been dropped: %+v", k, out)
		}
	}
}

// Both types keep the generic provisioner everywhere except List, which has to
// walk the instances because discovery names none and sqladmin has no wildcard.
func TestSslCertAndBackupRunListOverrides(t *testing.T) {
	if _, ok := registry.Get(SslCertResourceType, resource.OperationList, nil).(*sslCertListProvisioner); !ok {
		t.Error("SslCert List is not the parent-walking provisioner")
	}
	if _, ok := registry.Get(BackupRunResourceType, resource.OperationList, nil).(*backupRunListProvisioner); !ok {
		t.Error("BackupRun List is not the parent-walking provisioner")
	}
	for _, rt := range []string{SslCertResourceType, BackupRunResourceType} {
		for _, op := range []resource.Operation{
			resource.OperationCreate, resource.OperationRead, resource.OperationDelete,
		} {
			if !registry.HasProvisioner(rt, op) {
				t.Errorf("%s %v not registered", rt, op)
			}
		}
	}
}
