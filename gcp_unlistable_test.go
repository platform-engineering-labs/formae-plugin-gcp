// © 2026 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/platform-engineering-labs/formae/pkg/plugin"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"google.golang.org/api/googleapi"
)

// capturingLogger records what the plugin logged, at what level.
type capturingLogger struct{ lines *[]string }

func (c capturingLogger) log(level, msg string, attrs ...any) {
	parts := []string{level + " " + msg}
	for _, a := range attrs {
		parts = append(parts, fmt.Sprint(a))
	}
	*c.lines = append(*c.lines, strings.Join(parts, " "))
}
func (c capturingLogger) Debug(msg string, attrs ...any) { c.log("DEBUG", msg, attrs...) }
func (c capturingLogger) Info(msg string, attrs ...any)  { c.log("INFO", msg, attrs...) }
func (c capturingLogger) Warn(msg string, attrs ...any)  { c.log("WARN", msg, attrs...) }
func (c capturingLogger) Error(msg string, attrs ...any) { c.log("ERROR", msg, attrs...) }
func (c capturingLogger) With(attrs ...any) plugin.Logger {
	return c
}

func ctxWithLog(lines *[]string) context.Context {
	return plugin.WithLogger(context.Background(), capturingLogger{lines: lines})
}

func disabledErr() error {
	return fmt.Errorf("failed to list resources: %w", &googleapi.Error{
		Code:    403,
		Message: "GKE Hub API has not been used in project development-477117 before or it is disabled.",
		Details: []interface{}{
			map[string]interface{}{"reason": "SERVICE_DISABLED", "domain": "googleapis.com"},
		},
	})
}

func deniedErr(msg string) error {
	return fmt.Errorf("failed to list resources: %w", &googleapi.Error{Code: 403, Message: msg})
}

// A disabled API is an expected, benign state - no project enables all ~250
// registered types - so it is Info, and it says why nothing was listed.
func TestListLogsInfoAndEmptiesWhenTheAPIIsNotEnabled(t *testing.T) {
	var lines []string
	got, err := emptyIfUnlistable(ctxWithLog(&lines), "GCP::GKEHub::Membership", nil, disabledErr())

	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got == nil || len(got.NativeIDs) != 0 {
		t.Fatalf("result = %v, want an empty ListResult", got)
	}
	if len(lines) != 1 {
		t.Fatalf("logged %d lines, want 1: %v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "INFO ") {
		t.Errorf("logged at the wrong level: %q", lines[0])
	}
	for _, want := range []string{"not enabled", "GCP::GKEHub::Membership"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("log line %q does not mention %q", lines[0], want)
		}
	}
}

// Not being authorized to list is a real gap someone should close, so it is
// Warn - but it still must not fail the sync command, and it must say that
// the empty result means "could not look" rather than "nothing is there".
func TestListLogsWarnAndEmptiesWhenNotAuthorized(t *testing.T) {
	cases := map[string]error{
		"a plain permission denial":  deniedErr(`Required "container.clusters.list" permission(s) for "projects/p".`),
		"an end-user-only API":       deniedErr("Authentication error. Invalid end user or user type not supported."),
		"Cloud SQL's notAuthorized": deniedErr("The client is not authorized to make this request."),
	}
	for name, listErr := range cases {
		t.Run(name, func(t *testing.T) {
			var lines []string
			got, err := emptyIfUnlistable(ctxWithLog(&lines), "GCP::Container::Cluster", nil, listErr)

			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if got == nil || len(got.NativeIDs) != 0 {
				t.Fatalf("result = %v, want an empty ListResult", got)
			}
			if len(lines) != 1 || !strings.HasPrefix(lines[0], "WARN ") {
				t.Fatalf("logged %v, want one WARN line", lines)
			}
			if !strings.Contains(lines[0], "GCP::Container::Cluster") {
				t.Errorf("log line %q does not name the resource type", lines[0])
			}
		})
	}
}

// Anything else is a genuine failure and has to keep failing, silently as far
// as this guard is concerned - core logs and counts it.
func TestListPassesOtherOutcomesThrough(t *testing.T) {
	var lines []string
	boom := errors.New("failed to list resources: googleapi: Error 500: backend error")
	if _, err := emptyIfUnlistable(ctxWithLog(&lines), "GCP::Compute::Network", nil, boom); !errors.Is(err, boom) {
		t.Errorf("err = %v, want the original error", err)
	}
	if len(lines) != 0 {
		t.Errorf("logged %v, want nothing", lines)
	}

	ok := &resource.ListResult{NativeIDs: []string{"projects/p/networks/n"}}
	got, err := emptyIfUnlistable(ctxWithLog(&lines), "GCP::Compute::Network", ok, nil)
	if err != nil || got != ok {
		t.Errorf("success was altered: result=%v err=%v", got, err)
	}
	if len(lines) != 0 {
		t.Errorf("logged %v on success, want nothing", lines)
	}
}
