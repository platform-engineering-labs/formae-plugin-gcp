// © 2026 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package transport

import (
	"errors"

	"google.golang.org/api/googleapi"
)

// IsUserProjectDenied reports whether an error is Service Usage refusing the
// quota project named by X-Goog-User-Project, which every request this plugin
// sends carries, as opposed to the API refusing the call itself.
//
// The usual cause is a binding that has not propagated yet: Service Usage
// checks serviceusage.services.use against its own view of the project's IAM
// policy, which trails the policy by up to a few minutes. A principal granted
// roles/editor moments earlier (as `formae connect gcp` does) is refused here
// while the same grant already lets other calls through. Observed in the
// oidc-credential e2e against project formae-e2e-oidc on 2026-11-17, five
// seconds after the grant, with no audit entry for the refused call:
//
//	403 "Caller does not have required permission to use project ....
//	     Grant the caller the roles/serviceusage.serviceUsageConsumer role ...
//	     Propagation of the new permission may take a few minutes."
//	{"@type": ".../google.rpc.ErrorInfo",
//	 "reason": "USER_PROJECT_DENIED",
//	 "domain": "googleapis.com", ...}
//
// As with IsServiceDisabled, only the structured reason on a 403 counts.
func IsUserProjectDenied(err error) bool {
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
		if reason, ok := detail["reason"].(string); ok && reason == "USER_PROJECT_DENIED" {
			return true
		}
	}
	return false
}
