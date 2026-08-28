// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package base

import "testing"

func TestCheckLROStatus(t *testing.T) {
	tests := []struct {
		name    string
		op      map[string]interface{}
		done    bool
		wantErr bool
	}{
		{
			name: "not done",
			op:   map[string]interface{}{"done": false},
			done: false,
		},
		{
			name: "done, no error key",
			op:   map[string]interface{}{"done": true, "response": map[string]interface{}{}},
			done: true,
		},
		{
			// The bug this exists for: an empty error object is an absent
			// status, not a failure. Reporting it as one made formae retry a
			// create that had already succeeded, and the retry collided with
			// the resource the first attempt created.
			name: "done, error present but empty",
			op:   map[string]interface{}{"done": true, "error": map[string]interface{}{}},
			done: true,
		},
		{
			name: "done, error with code 0 and no message",
			op:   map[string]interface{}{"done": true, "error": map[string]interface{}{"code": float64(0)}},
			done: true,
		},
		{
			name:    "done, error with message",
			op:      map[string]interface{}{"done": true, "error": map[string]interface{}{"message": "boom"}},
			done:    true,
			wantErr: true,
		},
		{
			name:    "done, error with non-zero code only",
			op:      map[string]interface{}{"done": true, "error": map[string]interface{}{"code": float64(7)}},
			done:    true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done, err := CheckLROStatus(tt.op)
			if done != tt.done {
				t.Errorf("done = %v, want %v", done, tt.done)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// A code-only failure must still say something useful.
func TestCheckLROStatusCodeOnlyMessage(t *testing.T) {
	_, err := CheckLROStatus(map[string]interface{}{
		"done":  true,
		"error": map[string]interface{}{"code": float64(7)},
	})
	if err == nil || err.Error() != "operation failed with code 7" {
		t.Errorf("err = %v", err)
	}
}
