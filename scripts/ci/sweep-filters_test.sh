#!/usr/bin/env bash
# © 2025 Platform Engineering Labs Inc.
#
# SPDX-License-Identifier: FSL-1.1-ALv2
#
# "|" is not an OR in a gcloud --filter expression. Where gcloud filters
# client-side it is swallowed into the regex operand and the first alternative
# happens to match, so the sweep looks fine; where gcloud forwards the filter to
# the API - API Gateway, Service Directory - the call is rejected outright:
#
#   ERROR: (gcloud.api-gateway.apis.list) INVALID_ARGUMENT: invalid list filter
#
# clean-environment.sh sends every list to /dev/null and falls back to "", so a
# rejected filter prints "No API Gateway apis found" and sweeps nothing. That is
# indistinguishable from a clean project, and it ran for long enough to put 31
# API Gateway apis against a project cap of 50, which is what turned three
# conformance cases red every night with "Quota 'GlobalApisPerProject'
# exhausted".
#
# Use the project convention instead: list unfiltered and match client-side
# against SWEEP_RE, the way every other sweep in the file already does.
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
fail=0

while IFS= read -r hit; do
    echo "FAIL $hit"
    echo "     '|' is not an OR in a gcloud filter - list unfiltered and pipe through grep -E \"\$SWEEP_RE\""
    fail=1
done < <(grep -nE -- '--filter="[^"]*\|' "$here"/*.sh | grep -v "$(basename "$0")")

[ "$fail" -eq 0 ] && echo "ok   no gcloud --filter uses '|' as an OR"
exit "$fail"
