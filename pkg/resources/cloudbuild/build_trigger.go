// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package cloudbuild

import (
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// buildTriggerOutputOnly are the fields Cloud Build assigns and must never be
// sent back in a request body.
//
// They are declared in the schema (a read-back carries all three, so they would
// otherwise be unexpected fields) and stripped here.
//
// "resourceName" is the one that actually breaks things, and not in the way the
// usual "output-only field lands in the update mask" story predicts. Cloud
// Build authorizes a PATCH against the *body's* resourceName rather than the
// URL's: a PATCH to .../triggers/formae-probe-p2 carrying
// "resourceName": "projects/x/locations/global/triggers/y" was refused with
// HTTP 403 "The caller does not have permission", while the identical call
// without it succeeded. Since a stored resourceName names a uuid path this
// plugin never addresses, replaying it is a cross-project authorization check
// waiting to fail.
//
// "id" and "createTime" were verified harmless on both create and patch - the
// server ignores them - and are stripped anyway, because the only reason they
// are harmless is that Cloud Build happens to overwrite them.
var buildTriggerOutputOnly = []string{"id", "createTime", "resourceName"}

// expandServiceAccountRef turns the bare service-account email a forma declares
// (and an IAM ServiceAccount resolvable's `email` property yields) into the
// fully-qualified form the API requires.
//
// The short form is rejected: creating a trigger with
// "serviceAccount": "formae-tester@development-477117.iam.gserviceaccount.com"
// answers HTTP 400 INVALID_ARGUMENT, while
// "projects/development-477117/serviceAccounts/formae-tester@..." succeeds.
//
// A value that already contains a "/" is passed through untouched, so a forma
// can name a service account in another project explicitly. Note that such a
// value will still be shortened to the email on read - see
// shortenServiceAccountRef - so declare the email, not the path.
func expandServiceAccountRef(value, project string) string {
	if value == "" || strings.Contains(value, "/") {
		return value
	}
	return fmt.Sprintf("projects/%s/serviceAccounts/%s", project, value)
}

// shortenServiceAccountRef is the exact inverse: the API echoes the full path
// back, and a forma declares the email.
//
// Both halves have to exist. Expanding on the request without shortening on the
// response leaves the declared value and the stored state permanently
// disagreeing, and every re-apply then plans a change that is not one.
func shortenServiceAccountRef(value string) string {
	if i := strings.LastIndex(value, "/"); i >= 0 {
		return value[i+1:]
	}
	return value
}

// buildTriggerRequestTransformer strips the server-owned fields and expands the
// service-account reference.
//
// `name` deliberately stays in the body on both create and update. On create it
// is the client-chosen id (Cloud Build takes it in the body, not as a query
// parameter - omit it and the trigger is silently named the literal "trigger").
// On update it is an identity: a PATCH carrying the same name it is addressed by
// changes nothing, and one carrying a different name renames the trigger, which
// is why the schema marks it createOnly so a rename plans a replacement rather
// than stranding the native id.
//
// There is no update mask to worry about: the collection is configured with
// UpdateMaskFromBody off, because a maskless Cloud Build PATCH performs a full
// update of the mutable fields - verified live, a second PATCH omitting
// description, tags and substitutions cleared all three - which is exactly the
// reconcile semantics formae wants.
func buildTriggerRequestTransformer(
	props map[string]interface{}, ctx base.TransformContext,
) (map[string]interface{}, error) {
	drop := make(map[string]bool, len(buildTriggerOutputOnly))
	for _, f := range buildTriggerOutputOnly {
		drop[f] = true
	}

	out := make(map[string]interface{}, len(props))
	for k, v := range props {
		if drop[k] {
			continue
		}
		out[k] = v
	}
	if sa, ok := out["serviceAccount"].(string); ok {
		out["serviceAccount"] = expandServiceAccountRef(sa, ctx.Project)
	}
	return out, nil
}

// buildTriggerResponseTransformer shortens the service-account path back to the
// declared email and restores the two boolean defaults Cloud Build drops.
//
// The drops were both observed against the live API and both would read as
// drift on a forma that declared the value explicitly:
//
//   - "disabled": false sent, absent from the response. Restored here, and the
//     schema also marks it hasProviderDefault so a forma that never mentions it
//     tolerates the false this puts in state.
//   - "approvalConfig": {"approvalRequired": false} sent, echoed back as the
//     empty object {}. Restored only when the enclosing approvalConfig is
//     present, which is what makes this safe: approvalConfig has exactly one
//     field, so its presence implies the field, and a forma that omits the
//     whole block gets nothing injected. A nested field cannot carry
//     hasProviderDefault - the hint reaches only top-level fields - so the
//     repair has to happen here.
//
// This is also why the schema declares no other optional nested boolean. Cloud
// Build drops every false and every zero-valued enum inside `build` too
// (options.machineType "UNSPECIFIED", options.requestedVerifyOption
// "NOT_VERIFIED", options.logStreamingOption "STREAM_DEFAULT",
// steps[].allowFailure false all vanished from the read-back), and none of
// those sit in a single-field object, so there is no presence signal to hang a
// repair on.
func buildTriggerResponseTransformer(
	apiResponse map[string]interface{}, _ base.TransformContext,
) map[string]interface{} {
	if sa, ok := apiResponse["serviceAccount"].(string); ok {
		apiResponse["serviceAccount"] = shortenServiceAccountRef(sa)
	}
	if _, ok := apiResponse["disabled"]; !ok {
		apiResponse["disabled"] = false
	}
	if ac, ok := apiResponse["approvalConfig"].(map[string]interface{}); ok {
		if _, ok := ac["approvalRequired"]; !ok {
			ac["approvalRequired"] = false
		}
	}
	return apiResponse
}
