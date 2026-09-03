#!/usr/bin/env bash
# © 2025 Platform Engineering Labs Inc.
#
# SPDX-License-Identifier: FSL-1.1-ALv2
#
# Delete the Analytics Hub resources the conformance suite leaves behind.
#
# Two reasons this cannot ride along with the generic sweep in
# clean-environment.sh: gcloud has no analytics-hub surface, and Analytics Hub
# ids allow only letters, digits and underscores - so these resources are named
# with underscores ("formae_test_le_...") and the hyphenated grep every other
# sweep uses would never match them.
set -uo pipefail

PROJECT="${GCP_PROJECT_ID:-$(gcloud config get-value project 2>/dev/null || true)}"
LOCATION="${GCP_LOCATION:-}"
TOKEN="$(gcloud auth print-access-token 2>/dev/null || true)"
if [ -z "$PROJECT" ] || [ -z "$LOCATION" ] || [ -z "$TOKEN" ]; then
    echo "clean-analyticshub: no project, location or token, skipping"
    exit 0
fi

api="https://analyticshub.googleapis.com/v1"
base="${api}/projects/${PROJECT}/locations/${LOCATION}"

names_in() { # full collection URL, items key -> test-owned resource names
    curl -s -H "Authorization: Bearer ${TOKEN}" "$1" \
        | grep -o '"name": *"[^"]*"' \
        | sed -E 's/.*"(projects\/[^"]*)".*/\1/' \
        | grep -E 'formae_(test|probe|plugin)_' || true
}

delete() {
    echo "  Deleting $(basename "$1")"
    curl -s -X DELETE -H "Authorization: Bearer ${TOKEN}" "${api}/$1" >/dev/null || true
}

echo "Cleaning GCP analytics hub resources..."
for exchange in $(names_in "${base}/dataExchanges" ); do
    # Listings and query templates first: an exchange holding either cannot be
    # deleted.
    for coll in listings queryTemplates; do
        for nested in $(names_in "${api}/${exchange}/${coll}"); do
            delete "$nested"
        done
    done
    delete "$exchange"
done
echo "clean-analyticshub: done"
