// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package compute

import "testing"

// Compute parks some rejections - a quota refusal in particular - at
// status=RUNNING with the error already attached and never advances them to
// DONE. An operation carrying errors is finished whatever its status says;
// waiting for DONE means polling until the caller's timeout with nothing in the
// log naming the cause.
func TestCheckComputeOperationStatus(t *testing.T) {
	tests := []struct {
		name     string
		response map[string]interface{}
		wantDone bool
		wantErr  string
	}{
		{
			name:     "running",
			response: map[string]interface{}{"status": "RUNNING"},
		},
		{
			name:     "done",
			response: map[string]interface{}{"status": "DONE"},
			wantDone: true,
		},
		{
			name: "quota refusal parked at RUNNING",
			response: map[string]interface{}{
				"status":   "RUNNING",
				"progress": float64(0),
				"error": map[string]interface{}{"errors": []interface{}{
					map[string]interface{}{
						"code":    "QUOTA_EXCEEDED",
						"message": "Quota 'SSL_CERTIFICATES' exceeded.  Limit: 10.0 globally.",
					},
				}},
			},
			wantDone: true,
			wantErr:  "operation failed: Quota 'SSL_CERTIFICATES' exceeded.  Limit: 10.0 globally.",
		},
		{
			name: "error on a DONE operation",
			response: map[string]interface{}{
				"status": "DONE",
				"error": map[string]interface{}{"errors": []interface{}{
					map[string]interface{}{"message": "boom"},
				}},
			},
			wantDone: true,
			wantErr:  "operation failed: boom",
		},
		{
			// An empty error object on a running operation is not an answer.
			name:     "empty error object while running",
			response: map[string]interface{}{"status": "RUNNING", "error": map[string]interface{}{}},
		},
		{
			name:     "empty error object once done",
			response: map[string]interface{}{"status": "DONE", "error": map[string]interface{}{}},
			wantDone: true,
			wantErr:  "operation failed with unknown error",
		},
		{
			name:     "no status field",
			response: map[string]interface{}{},
			wantErr:  "operation response missing status field",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done, err := checkComputeOperationStatus(tt.response)
			if done != tt.wantDone {
				t.Errorf("done = %v, want %v", done, tt.wantDone)
			}
			got := ""
			if err != nil {
				got = err.Error()
			}
			if got != tt.wantErr {
				t.Errorf("err = %q, want %q", got, tt.wantErr)
			}
		})
	}
}
