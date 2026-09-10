// © 2026 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package transport

import (
	"errors"
	"strings"

	"google.golang.org/api/googleapi"
)

// endUserOnlyMessage is Google's answer when a call is restricted to identities
// that can own the thing being asked for - a human, or a service account - and
// the caller is neither.
//
// A workload-identity-federation principal is neither. The agent on a hosted
// installation authenticates as one: the roles are bound straight to
// "principal://iam.googleapis.com/.../workloadIdentityPools/.../subject/..."
// with no service account impersonated anywhere in the chain, so Google sees a
// principal type this class of API refuses to resolve.
const endUserOnlyMessage = "Invalid end user or user type not supported"

// IsEndUserOnly reports whether an error says the caller is the wrong kind of
// identity for the call, as opposed to lacking permission for it.
//
// The distinction is not academic, and it is why no amount of IAM fixes this.
// Cloud Logging's savedQueries.list is the case that forced it: listing private
// saved queries needs "logging.queries.list", and that permission is in no role
// Google publishes - not logging.admin, not editor, not owner (13,702
// permissions, checked). A private saved query belongs to a person, so there is
// nothing to grant. A service account calling the same URL answers 200 and
// simply owns none; the federated principal cannot be asked the question at
// all.
//
// Matched on the message rather than a structured reason, which is worth being
// honest about. The agent's own error formatting drops googleapi.Error.Details
// before anything is logged - the disabled-API errors from the same
// installation carry three details and log none of them - so the production
// logs cannot say whether this answer has a machine-readable reason, and
// reproducing it needs a federated credential this repo has no way to mint.
// The sentence is a fixed Google string and the status is checked alongside it.
// The package already takes this trade deliberately elsewhere: isRetryableSQLError
// matches "is being accessed by other users" the same way. Replace this with
// the structured reason the first time anyone can capture one.
func IsEndUserOnly(err error) bool {
	if err == nil {
		return false
	}
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) || gerr.Code != 403 {
		return false
	}
	return strings.Contains(gerr.Message, endUserOnlyMessage)
}
