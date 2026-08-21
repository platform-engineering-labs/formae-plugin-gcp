// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import "testing"

const signedUrlKeyPath = "projects/dev-1/global/backendServices/bs-1/signedUrlKeys/key-a"

// A key is addressed by (backend service, key name); both must survive the
// round-trip, since Read and Delete rebuild the verb URL from the id.
func TestSignedUrlKeyNativeIDRoundTrip(t *testing.T) {
	if got := buildSignedUrlKeyNativeID("dev-1", "bs-1", "key-a"); got != signedUrlKeyPath {
		t.Fatalf("build: %q", got)
	}
	project, backendService, keyName, err := parseSignedUrlKeyNativeID(signedUrlKeyPath)
	if err != nil {
		t.Fatal(err)
	}
	if project != "dev-1" || backendService != "bs-1" || keyName != "key-a" {
		t.Errorf("parse: %q %q %q", project, backendService, keyName)
	}
}

func TestParseSignedUrlKeyNativeIDRejectsOtherShapes(t *testing.T) {
	for _, bad := range []string{
		"projects/dev-1/global/backendServices/bs-1",                    // the service itself
		"projects/dev-1/global/backendServices/bs-1/signedUrlKeys/",     // no key name
		"projects/dev-1/regions/r/backendServices/bs-1/signedUrlKeys/k", // regional shape
		"projects/dev-1/global/backendBuckets/bb-1/signedUrlKeys/k",     // wrong collection
		"",
	} {
		if _, _, _, err := parseSignedUrlKeyNativeID(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

// The API reports only key names, under cdnPolicy.signedUrlKeyNames, and omits
// the block entirely when no keys exist — so presence is all a read can check.
func TestSignedUrlKeyPresence(t *testing.T) {
	withKeys := map[string]interface{}{
		"cdnPolicy": map[string]interface{}{
			"signedUrlKeyNames": []interface{}{"key-a", "key-b"},
		},
	}
	if !signedUrlKeyPresent(withKeys, "key-b") {
		t.Error("key-b should be present")
	}
	if signedUrlKeyPresent(withKeys, "key-c") {
		t.Error("key-c should be absent")
	}
	if got := signedUrlKeyNames(withKeys); len(got) != 2 {
		t.Errorf("names: %#v", got)
	}

	// A backend service with CDN on but no keys has no signedUrlKeyNames at all,
	// and one with CDN off has no cdnPolicy — neither may panic or report a key.
	for _, empty := range []map[string]interface{}{
		{"cdnPolicy": map[string]interface{}{"cacheMode": "CACHE_ALL_STATIC"}},
		{"name": "bs-1"},
		{"cdnPolicy": "not-an-object"},
		{"cdnPolicy": map[string]interface{}{"signedUrlKeyNames": []interface{}{"", 42}}},
	} {
		if signedUrlKeyPresent(empty, "key-a") {
			t.Errorf("no key should be reported for %#v", empty)
		}
		if got := signedUrlKeyNames(empty); len(got) != 0 {
			t.Errorf("expected no names for %#v, got %#v", empty, got)
		}
	}
}
