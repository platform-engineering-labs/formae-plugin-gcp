// © 2026 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package base

import (
	"context"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/transport"
	"github.com/platform-engineering-labs/formae/pkg/plugin"
)

// logUnauthorizedRead says what the API said when a read is refused on
// authorization, and stays quiet for every other failure.
//
// The read result carries an error *code* back to core, never a message, and
// core's own record of a terminal failure is
//
//	ResourceUpdater: counting terminal failure type=... operation=read stage=...
//
// with no URL, no status and no text, at any log level. So a refused read is
// the one failure nothing in the system can explain: sixteen Cloud SQL
// databases failed this way on a production installation for over a day, 542
// times in 24 hours, and learning that the cause was a 403 masking a deleted
// parent instance took reproducing the call by hand against live GCP. This
// closes that, on the one class of failure where the API's words are the whole
// answer.
//
// Warn, not Error: core already reports the failure and counts it, and a second
// ERROR voice per resource per cycle is what buried the signal in the first
// place. Restricted to 401 and 403 for the same reason - a message on every
// 404 would drown the one that carries information.
//
// The read still fails. This only makes it legible; whether a refusal means the
// resource is gone is a separate question, answered per type by
// ResourceConfig.ReadErrorTreatAsMissing.
func logUnauthorizedRead(ctx context.Context, url string, err error) {
	if err == nil || transport.ClassifyError(err) != transport.ErrorCodeUnauthorized {
		return
	}
	plugin.LoggerFromContext(ctx).Warn(
		"read refused: not authorized",
		"url", url,
		"error", err.Error(),
	)
}
