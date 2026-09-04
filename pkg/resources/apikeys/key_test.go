// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package apikeys

import (
	"reflect"
	"strings"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// The one test in this file that is about safety rather than correctness: the
// completed create operation's payload carries the live key string, and it must
// not survive into properties.
func TestKeyResponseTransformerDropsTheCredential(t *testing.T) {
	in := map[string]interface{}{
		"name":        "projects/989754770009/locations/global/keys/formae-test-ak-key-abcd1234",
		"displayName": "Formae conformance test key",
		"keyString":   "AIzaSyNOTAREALKEYSTRINGATALL0000000000000",
		"uid":         "f730ceec-9838-4b43-984b-5a962c1a161e",
		"createTime":  "2026-09-03T15:08:16.101326Z",
		"updateTime":  "2026-09-03T15:08:16.204091Z",
		"etag":        `W/"4oRU7NLgLc30yd2S3wH72A=="`,
		"annotations": map[string]interface{}{"environment": "test"},
		"restrictions": map[string]interface{}{
			"apiTargets": []interface{}{
				map[string]interface{}{"service": "translate.googleapis.com"},
			},
			"serverKeyRestrictions": map[string]interface{}{
				"allowedIps": []interface{}{"203.0.113.4"},
			},
		},
	}
	want := map[string]interface{}{
		"name":        "formae-test-ak-key-abcd1234",
		"displayName": "Formae conformance test key",
		"annotations": map[string]interface{}{"environment": "test"},
		"restrictions": map[string]interface{}{
			"apiTargets": []interface{}{
				map[string]interface{}{"service": "translate.googleapis.com"},
			},
			"serverKeyRestrictions": map[string]interface{}{
				"allowedIps": []interface{}{"203.0.113.4"},
			},
		},
	}
	got := keyResponseTransformer(in, base.TransformContext{})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
	for k, v := range got {
		if s, ok := v.(string); ok && strings.HasPrefix(s, "AIza") {
			t.Errorf("field %q still holds key material", k)
		}
	}
}

// A key bound to a service account has a key string with a different prefix
// ("AQ.Ab8..." rather than "AIza..."), so the drop has to be by field name and
// not by any guess at the value's shape.
func TestKeyResponseTransformerDropsAnyKeyStringShape(t *testing.T) {
	in := map[string]interface{}{
		"name":      "projects/989754770009/locations/global/keys/k",
		"keyString": "AQ.Ab8RN0TAREALKEYSTRING0000000000000000000000",
	}
	got := keyResponseTransformer(in, base.TransformContext{})
	if _, present := got["keyString"]; present {
		t.Fatal("keyString survived")
	}
}

// keys.create returns the key string and keys.get does not. Both reads have to
// transform to the same properties, or a resource verified straight after
// create disagrees with the same resource on the next sync.
func TestKeyResponseTransformerCreateAndGetAgree(t *testing.T) {
	create := map[string]interface{}{
		"name":        "projects/989754770009/locations/global/keys/k",
		"displayName": "d",
		"keyString":   "AIzaSyNOTAREALKEYSTRING000000000000000000",
		"uid":         "f730ceec-9838-4b43-984b-5a962c1a161e",
		"createTime":  "2026-09-03T15:08:16.101326Z",
		"updateTime":  "2026-09-03T15:08:16.204091Z",
		"etag":        `W/"4oRU7NLgLc30yd2S3wH72A=="`,
	}
	get := map[string]interface{}{
		"name":        "projects/989754770009/locations/global/keys/k",
		"displayName": "d",
		"uid":         "f730ceec-9838-4b43-984b-5a962c1a161e",
		"createTime":  "2026-09-03T15:08:16.101326Z",
		"updateTime":  "2026-09-03T15:08:16.204091Z",
		"etag":        `W/"1j9V6zf6NP/CqFiB/gHejQ=="`, // recomputed between the two reads
	}
	a := keyResponseTransformer(create, base.TransformContext{})
	b := keyResponseTransformer(get, base.TransformContext{})
	if !reflect.DeepEqual(a, b) {
		t.Errorf("create %#v\nget    %#v", a, b)
	}
}

// A soft-deleted key answers a read with HTTP 200 for thirty days. Only
// deleteTime distinguishes it from a live one.
func TestKeyDeleted(t *testing.T) {
	cases := []struct {
		name string
		body map[string]interface{}
		want bool
	}{
		{"live key", map[string]interface{}{"name": "k"}, false},
		{"empty deleteTime is not a tombstone", map[string]interface{}{"deleteTime": ""}, false},
		{"tombstone", map[string]interface{}{"deleteTime": "2026-09-03T15:09:29.253812Z"}, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := keyDeleted(tt.body); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// The API reports every name in project-NUMBER form; the native ID has to come
// out in project-ID form, whether it was built from a create, a read or a list
// item. Otherwise the same key is managed under one identity and discovered
// under another.
func TestExtractAPIKeyNativeIDNormalisesTheProject(t *testing.T) {
	ctx := base.PathContext{Project: "development-477117", ResourceType: keyCollection}
	want := "projects/development-477117/locations/global/keys/formae-test-ak-key-abcd1234"

	// A list item, or a read: name is present, in project-number form.
	listItem := map[string]interface{}{
		"name": "projects/989754770009/locations/global/keys/formae-test-ak-key-abcd1234",
	}
	if got := extractAPIKeyNativeID(listItem, ctx); got != want {
		t.Errorf("list item: got %q, want %q", got, want)
	}

	// A create: the response is an Operation, and the id comes from context.
	createCtx := ctx
	createCtx.ResourceName = "formae-test-ak-key-abcd1234"
	op := map[string]interface{}{"name": "operations/akmf.p7-989754770009-9987c969"}
	if got := extractAPIKeyNativeID(op, createCtx); got != want {
		t.Errorf("create: got %q, want %q", got, want)
	}
}

// An Operation name is not a resource path and must never be mistaken for one.
func TestExtractAPIKeyNativeIDRejectsAnOperationWithoutContext(t *testing.T) {
	op := map[string]interface{}{"name": "operations/akmf.p7-989754770009-9987c969"}
	if got := extractAPIKeyNativeID(op, base.PathContext{}); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// build -> parse -> build must be the identity, and the parsed context must
// address the same URL the id names.
func TestAPIKeyNativeIDRoundTripIsIdentity(t *testing.T) {
	const nativeID = "projects/development-477117/locations/global/keys/formae-test-ak-key-abcd1234"
	ctx, err := parseAPIKeyNativeID(nativeID)
	if err != nil {
		t.Fatal(err)
	}
	if got := extractAPIKeyNativeID(map[string]interface{}{}, ctx); got != nativeID {
		t.Errorf("got %q, want %q", got, nativeID)
	}
	if got := apiKeysPathBuilder(ctx); got != "/"+nativeID {
		t.Errorf("path builder gave %q, want %q", got, "/"+nativeID)
	}
}

func TestParseAPIKeyNativeIDRejectsGarbage(t *testing.T) {
	for _, bad := range []string{
		"",
		"keys/k",
		"projects/p/locations/global/keys",
		"projects/p/locations/global/notkeys/k",
		"projects/p/keys/k",
		"projects/p/locations/global/keys/k/extra",
	} {
		if _, err := parseAPIKeyNativeID(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

// A create that is going to fail answers HTTP 200 with an accepted operation
// and reports the failure only once the operation completes. The status checker
// is the only thing standing between that and a create reported as a success.
func TestCheckOperationStatus(t *testing.T) {
	if done, err := checkOperationStatus(map[string]interface{}{}); done || err != nil {
		t.Errorf("pending: got done=%v err=%v", done, err)
	}
	done, err := checkOperationStatus(map[string]interface{}{
		"done": true,
		"error": map[string]interface{}{
			"code":    float64(6),
			"message": "A key with this id already exists.",
		},
	})
	if !done || err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("failed op: got done=%v err=%v", done, err)
	}
	if done, err := checkOperationStatus(map[string]interface{}{"done": true}); !done || err != nil {
		t.Errorf("success: got done=%v err=%v", done, err)
	}
}

func TestExtractOperationName(t *testing.T) {
	if got := extractOperationName(map[string]interface{}{
		"name": "operations/akmf.p7-989754770009-9987c969",
	}); got != "operations/akmf.p7-989754770009-9987c969" {
		t.Errorf("got %q", got)
	}
	// A resource path is not an operation.
	if got := extractOperationName(map[string]interface{}{
		"name": "projects/989754770009/locations/global/keys/k",
	}); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
