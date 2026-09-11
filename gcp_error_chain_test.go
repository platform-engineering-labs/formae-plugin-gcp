// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// A List that walks a parent collection first (instances before databases,
// routers before NATs, zones before record sets) reports the parent's failure
// through the same seam as any other List, and emptyIfUnlistable decides from
// the error's structure whether that failure means "nothing here": a disabled
// API is SERVICE_DISABLED in a googleapi.Error's details, a refused principal
// is its status code. Rendering the error to a string on the way out erases
// both, and the walked list fails every discovery cycle for a condition the
// direct lists already answer with an empty result and a log line.
//
// transport.WrapError keeps the underlying error reachable through Unwrap, so
// returning it is enough. This test names every site that flattens one instead.
var flattenedTransportError = regexp.MustCompile(`fmt\.Errorf\("%s", \w+\.Message\)`)

func TestWalkedListsKeepTheAPIErrorReachable(t *testing.T) {
	var offenders []string
	err := filepath.WalkDir("pkg/resources", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(src), "\n") {
			if flattenedTransportError.MatchString(line) {
				offenders = append(offenders, path+":"+strconv.Itoa(i+1))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Errorf("%d site(s) render a transport error to a string, which hides the API's answer from emptyIfUnlistable; return the wrapped error instead:\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}
