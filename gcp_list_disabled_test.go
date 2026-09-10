// © 2026 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"google.golang.org/api/googleapi"
)

func disabledAPIError() error {
	return fmt.Errorf("failed to list resources: %w", &googleapi.Error{
		Code:    403,
		Message: "GKE Hub API has not been used in project development-477117 before or it is disabled.",
		Details: []interface{}{
			map[string]interface{}{"reason": "SERVICE_DISABLED", "domain": "googleapis.com"},
		},
	})
}

// A project that has never turned an API on holds no resources of its types,
// which is what an empty list says. Reporting it as an error instead put two
// ERROR lines per discovery cycle into a production installation's logs and
// alerted on them, for a condition no code change can fix.
func TestListAnswersEmptyWhenTheAPIIsDisabled(t *testing.T) {
	result, err := emptyIfUnlistable(nil, disabledAPIError())
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if result == nil {
		t.Fatal("result = nil, want an empty ListResult")
	}
	if len(result.NativeIDs) != 0 {
		t.Errorf("NativeIDs = %v, want empty", result.NativeIDs)
	}
	if result.NextPageToken != nil {
		t.Errorf("NextPageToken = %v, want nil", result.NextPageToken)
	}
}

// Everything else has to travel unchanged, or a genuine failure becomes a
// silent "no resources exist" and formae reconciles live infrastructure away.
func TestListPassesEverythingElseThrough(t *testing.T) {
	otherErr := errors.New("failed to list resources: googleapi: Error 403: forbidden")
	if _, err := emptyIfUnlistable(nil, otherErr); !errors.Is(err, otherErr) {
		t.Errorf("err = %v, want the original error", err)
	}

	ok := &resource.ListResult{NativeIDs: []string{"projects/p/things/a"}}
	got, err := emptyIfUnlistable(ok, nil)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got != ok {
		t.Errorf("result = %v, want the original result", got)
	}
}

// The other answer that means "you will never enumerate this", for the same
// reason: nothing about the caller can change, so an error repeats forever.
func TestListAnswersEmptyWhenTheAPIWantsAnEndUser(t *testing.T) {
	err := fmt.Errorf("failed to list resources: %w", &googleapi.Error{
		Code:    403,
		Message: "Authentication error. Invalid end user or user type not supported.",
	})
	result, gotErr := emptyIfUnlistable(nil, err)
	if gotErr != nil {
		t.Fatalf("err = %v, want nil", gotErr)
	}
	if result == nil || len(result.NativeIDs) != 0 {
		t.Errorf("result = %v, want an empty ListResult", result)
	}
}
