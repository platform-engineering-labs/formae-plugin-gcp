// © 2026 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package base

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/platform-engineering-labs/formae/pkg/plugin"
	"google.golang.org/api/googleapi"
)

type readCapturingLogger struct{ lines *[]string }

func (c readCapturingLogger) log(level, msg string, attrs ...any) {
	parts := []string{level + " " + msg}
	for _, a := range attrs {
		parts = append(parts, fmt.Sprint(a))
	}
	*c.lines = append(*c.lines, strings.Join(parts, " "))
}
func (c readCapturingLogger) Debug(msg string, a ...any)   { c.log("DEBUG", msg, a...) }
func (c readCapturingLogger) Info(msg string, a ...any)    { c.log("INFO", msg, a...) }
func (c readCapturingLogger) Warn(msg string, a ...any)    { c.log("WARN", msg, a...) }
func (c readCapturingLogger) Error(msg string, a ...any)   { c.log("ERROR", msg, a...) }
func (c readCapturingLogger) With(a ...any) plugin.Logger  { return c }

// A read that fails on authorization is the one failure core cannot explain on
// its own: its terminal-failure record carries the type and nothing else - no
// URL, no status, no message - so the plugin has to say what the API said.
func TestLogUnauthorizedRead(t *testing.T) {
	url := "https://sqladmin.googleapis.com/v1/projects/p/instances/i/databases/d"

	t.Run("a 403 is logged with the API's own words", func(t *testing.T) {
		var lines []string
		ctx := plugin.WithLogger(context.Background(), readCapturingLogger{lines: &lines})

		logUnauthorizedRead(ctx, url, &googleapi.Error{
			Code:    403,
			Message: "The client is not authorized to make this request.",
		})

		if len(lines) != 1 || !strings.HasPrefix(lines[0], "WARN ") {
			t.Fatalf("logged %v, want one WARN line", lines)
		}
		for _, want := range []string{url, "not authorized", "not authorized to make this request"} {
			if !strings.Contains(lines[0], want) {
				t.Errorf("log line %q does not mention %q", lines[0], want)
			}
		}
	})

	t.Run("a 401 counts too", func(t *testing.T) {
		var lines []string
		ctx := plugin.WithLogger(context.Background(), readCapturingLogger{lines: &lines})
		logUnauthorizedRead(ctx, url, &googleapi.Error{Code: 401, Message: "invalid credentials"})
		if len(lines) != 1 {
			t.Errorf("logged %v, want one line", lines)
		}
	})

	// Everything else is core's to report; a second voice on every 404 would
	// bury the one message that carries information.
	t.Run("anything else is left alone", func(t *testing.T) {
		for _, err := range []error{
			&googleapi.Error{Code: 404, Message: "not found"},
			&googleapi.Error{Code: 500, Message: "backend error"},
			errors.New("connection refused"),
			nil,
		} {
			var lines []string
			ctx := plugin.WithLogger(context.Background(), readCapturingLogger{lines: &lines})
			logUnauthorizedRead(ctx, url, err)
			if len(lines) != 0 {
				t.Errorf("err %v logged %v, want nothing", err, lines)
			}
		}
	})
}
