#!/usr/bin/env bash
# © 2025 Platform Engineering Labs Inc.
#
# SPDX-License-Identifier: FSL-1.1-ALv2
#
# Delete the prerequisites one conformance case leaves behind, between its two
# phases.
#
# Conformance Destroy only removes the resource under test, so a case that needs
# a prerequisite leaves it alive when the crud phase ends. The discovery phase
# then builds the same prerequisite again under the same name - the fixtures key
# off one test-run id - and either collides with it ("The resource ... already
# exists") or runs the pair against a quota that only fits one.
#
# Each case owns a distinct name prefix, so a job clears only what it created.
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"

case "${1:-}" in
    alloydb-*)  exec "$here/clean-alloydb-case.sh"  "$1" ;;
    eventarc-*) exec "$here/clean-eventarc-case.sh" "$1" ;;
    security-policy-rule)        PREFIX="formae-plugin-sdk-test-spr-"  ;;
    region-security-policy-rule) PREFIX="formae-plugin-sdk-test-rspr-" ;;
    *)
        echo "clean-case-prereqs: nothing to do for '${1:-}'"
        exit 0
        ;;
esac

echo "Cleaning Cloud Armor policies named ${PREFIX}* ..."
POLICIES=$(gcloud compute security-policies list --format="value(name,region.basename())" 2>/dev/null \
    | grep "^${PREFIX}" || true)

if [ -z "$POLICIES" ]; then
    echo "  none found"
    exit 0
fi

echo "$POLICIES" | while read -r pol region; do
    if [ -n "$region" ]; then
        echo "  Deleting regional security policy: $pol (region: $region)"
        gcloud compute security-policies delete "$pol" --region="$region" --quiet 2>&1 | tail -1 || true
    else
        echo "  Deleting global security policy: $pol"
        gcloud compute security-policies delete "$pol" --global --quiet 2>&1 | tail -1 || true
    fi
done
