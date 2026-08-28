#!/usr/bin/env bash
# © 2025 Platform Engineering Labs Inc.
#
# SPDX-License-Identifier: FSL-1.1-ALv2
#
# Delete the Certificate Authority Service resources the conformance suite
# leaves behind.
#
# This one matters more than the other sweeps: a certificate authority is
# BILLED for as long as it exists, at a rate set by its pool's tier. A CA
# leaked by a failed run keeps costing money until someone notices. Pools and
# templates are free, but a pool holding a CA cannot be deleted, so all three
# are swept in dependency order.
#
# Deleting a CA needs skipGracePeriod: without it the CA moves to state DELETED
# and lingers - and is still billed - for 30 days.
set -uo pipefail

PROJECT="${GCP_PROJECT_ID:-$(gcloud config get-value project 2>/dev/null || true)}"
LOCATION="${GCP_LOCATION:-}"
TOKEN="$(gcloud auth print-access-token 2>/dev/null || true)"
if [ -z "$PROJECT" ] || [ -z "$LOCATION" ] || [ -z "$TOKEN" ]; then
    echo "clean-privateca: no project, location or token, skipping"
    exit 0
fi

api="https://privateca.googleapis.com/v1"
base="${api}/projects/${PROJECT}/locations/${LOCATION}"

names_in() { # collection URL, items key -> test-owned resource names
    curl -s -H "Authorization: Bearer ${TOKEN}" "$1" \
        | grep -o "\"name\": *\"[^\"]*\"" \
        | sed -E 's/.*"(projects\/[^"]*)".*/\1/' \
        | grep "formae-test-" || true
}

echo "Cleaning GCP private CA resources..."

# Certificate authorities first: a pool holding one cannot be deleted. The "-"
# wildcard asks across every pool at once.
for ca in $(names_in "${base}/caPools/-/certificateAuthorities" ); do
    echo "  Deleting CA: $(basename "$ca")"
    curl -s -X DELETE -H "Authorization: Bearer ${TOKEN}" \
        "${api}/${ca}?skipGracePeriod=true&ignoreActiveCertificates=true&ignoreDependentResources=true" \
        >/dev/null || true
done

# The CA deletes are long-running; a pool still holding one will refuse to go.
for _ in $(seq 1 30); do
    [ -z "$(names_in "${base}/caPools/-/certificateAuthorities")" ] && break
    sleep 10
done

for coll in caPools certificateTemplates; do
    for name in $(names_in "${base}/${coll}"); do
        echo "  Deleting $(basename "$name") ($coll)"
        curl -s -X DELETE -H "Authorization: Bearer ${TOKEN}" "${api}/${name}" >/dev/null || true
    done
done
echo "clean-privateca: done"
