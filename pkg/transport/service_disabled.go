// © 2026 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package transport

import (
	"errors"

	"google.golang.org/api/googleapi"
)

// IsServiceDisabled reports whether an error says the API itself has never been
// turned on for this project, as opposed to the caller being refused something
// the API does offer.
//
// GCP answers a call to a disabled service with 403 PERMISSION_DENIED, which is
// indistinguishable from a real authorization failure by status alone. The
// machine-readable answer is in the error's Details, not in Errors - captured
// live from gkehub.googleapis.com against project development-477117 on
// 2026-09-10, where Errors was empty and Details carried:
//
//	{"@type": ".../google.rpc.ErrorInfo",
//	 "reason": "SERVICE_DISABLED",
//	 "domain": "googleapis.com",
//	 "metadata": {"service": "gkehub.googleapis.com", ...}}
//
// The older envelope some APIs still send says the same thing as
// Errors[].Reason == "accessNotConfigured", so both are accepted.
//
// The status is checked as well as the reason. A reason string alone is not
// enough to turn an error into "nothing here", and neither is a message that
// merely mentions the words - only the structured field on a 403 counts.
func IsServiceDisabled(err error) bool {
	if err == nil {
		return false
	}
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) || gerr.Code != 403 {
		return false
	}
	for _, raw := range gerr.Details {
		detail, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if reason, ok := detail["reason"].(string); ok && reason == "SERVICE_DISABLED" {
			return true
		}
	}
	for _, item := range gerr.Errors {
		if item.Reason == "accessNotConfigured" {
			return true
		}
	}
	return false
}
