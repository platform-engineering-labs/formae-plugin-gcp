#!/usr/bin/env bash
# © 2025 Platform Engineering Labs Inc.
#
# SPDX-License-Identifier: FSL-1.1-ALv2
#
# Delete the Datastream private connections a conformance case leaves behind.
#
# A private connection reserves a /29 for Datastream, and the range stays
# reserved until the connection is really gone. The route case builds one as a
# prerequisite and the harness spares prerequisites on Destroy, so it outlives
# the crud phase - and the discovery phase, which declares the same /29, then
# fails with "The IP range specified overlaps with a reserved IP range".
#
# Streams and routes are removed first: a private connection in use will not go.
set -uo pipefail

PROJECT="${GCP_PROJECT_ID:-$(gcloud config get-value project 2>/dev/null || true)}"
LOCATION="${GCP_LOCATION:-}"
TOKEN="$(gcloud auth print-access-token 2>/dev/null || true)"
if [ -z "$PROJECT" ] || [ -z "$LOCATION" ] || [ -z "$TOKEN" ]; then
    echo "clean-datastream-case: no project, location or token, skipping"
    exit 0
fi

api="https://datastream.googleapis.com/v1"
base="${api}/projects/${PROJECT}/locations/${LOCATION}"

names_in() { # collection URL -> test-owned resource names
    curl -s -H "Authorization: Bearer ${TOKEN}" "$1" \
        | grep -o "\"name\": *\"[^\"]*\"" \
        | sed -E 's/.*"(projects\/[^"]*)".*/\1/' \
        | grep "formae-test-" || true
}

echo "Cleaning GCP datastream resources..."

for stream in $(names_in "${base}/streams"); do
    echo "  Deleting stream: $(basename "$stream")"
    curl -s -X DELETE -H "Authorization: Bearer ${TOKEN}" "${api}/${stream}" >/dev/null || true
done

for pc in $(names_in "${base}/privateConnections"); do
    for route in $(names_in "${api}/${pc}/routes"); do
        echo "  Deleting route: $(basename "$route")"
        curl -s -X DELETE -H "Authorization: Bearer ${TOKEN}" "${api}/${route}" >/dev/null || true
    done
    echo "  Deleting private connection: $(basename "$pc")"
    curl -s -X DELETE -H "Authorization: Bearer ${TOKEN}" "${api}/${pc}?force=true" >/dev/null || true
done

# Tearing down a VPC peering takes minutes, and the range stays reserved until
# it lands. The next phase declares the same /29, so wait rather than race it.
for _ in $(seq 1 60); do
    [ -z "$(names_in "${base}/privateConnections")" ] && break
    sleep 10
done

for cp in $(names_in "${base}/connectionProfiles"); do
    echo "  Deleting connection profile: $(basename "$cp")"
    curl -s -X DELETE -H "Authorization: Bearer ${TOKEN}" "${api}/${cp}" >/dev/null || true
done

# These deletes are long-running too. The discovery phase recreates profiles
# under the same names, and got "Resource ... already exists" when this raced.
for _ in $(seq 1 30); do
    [ -z "$(names_in "${base}/connectionProfiles")" ] && break
    sleep 10
done
echo "clean-datastream-case: done"
