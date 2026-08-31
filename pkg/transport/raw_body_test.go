// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package transport

import "testing"

// RawBody exists for the APIs that take content rather than a resource
// description. Sending it as JSON would corrupt the upload, and claiming
// application/json would store it with the wrong type.
func TestRequestOptionsCarriesRawBodyAndContentType(t *testing.T) {
	opts := RequestOptions{
		Method:      "POST",
		URL:         "https://example/upload",
		RawBody:     []byte("hello\n"),
		ContentType: "text/plain",
		// Body must be ignored when RawBody is set; both being populated is a
		// caller mistake that should not silently send the wrong one.
		Body: map[string]interface{}{"ignored": true},
	}
	if string(opts.RawBody) != "hello\n" {
		t.Errorf("RawBody = %q", string(opts.RawBody))
	}
	if opts.ContentType != "text/plain" {
		t.Errorf("ContentType = %q", opts.ContentType)
	}
}
