#!/usr/bin/env bash
# © 2025 Platform Engineering Labs Inc.
#
# SPDX-License-Identifier: FSL-1.1-ALv2
#
# Delete the Eventarc Advanced resources one conformance case leaves behind,
# or - with the argument "all" - every one this project has anywhere, which is
# what the whole-environment sweep calls.
#
# MessageBusesPerProjectPerRegion is 1. The pipeline case builds a bus as a
# prerequisite and the harness spares prerequisites on Destroy, so the bus
# outlives the crud phase and the discovery phase cannot create its own:
# "Quota limit 'MessageBusesPerProjectPerRegion' has been exceeded. Limit: 1".
#
# gcloud has no eventarc message-buses/pipelines surface, so this talks REST.
set -uo pipefail

# Every fixture pins its location: Eventarc Advanced is not available in every
# region, so none of them inherit the target's. And because the bus quota is 1
# per region, each case needs a region of its own - which is why this is a map
# and not a single default.
case "${1:-}" in
    eventarc-pipeline)           LOCATIONS="us-central1" ;;
    eventarc-message-bus)        LOCATIONS="europe-west1" ;;
    eventarc-enrollment)         LOCATIONS="us-east4" ;;
    eventarc-google-api-source)  LOCATIONS="europe-west3" ;;
    # Every region Eventarc Advanced supports (probed 2026-08-28 by POSTing
    # messageBuses?validateOnly=true to each candidate). Listing them all means
    # a new case's region is swept even before anyone adds it above.
    all) LOCATIONS="us-central1 us-east4 europe-west1 europe-west3 europe-north1 asia-southeast1 australia-southeast1" ;;
    *)
        echo "clean-eventarc-case: nothing to do for '${1:-}'"
        exit 0
        ;;
esac

PROJECT="${GCP_PROJECT_ID:-$(gcloud config get-value project 2>/dev/null || true)}"
TOKEN="$(gcloud auth print-access-token 2>/dev/null || true)"
if [ -z "$PROJECT" ] || [ -z "$TOKEN" ]; then
    echo "clean-eventarc-case: no project or token, skipping"
    exit 0
fi

api="https://eventarc.googleapis.com/v1/projects/${PROJECT}/locations"

names_in() { # collection location -> full resource names of test leftovers
    curl -s -H "Authorization: Bearer ${TOKEN}" "${api}/$2/$1" \
        | grep -o '"name": *"[^"]*"' \
        | sed -E 's/.*"(projects\/[^"]*)".*/\1/' \
        | grep "formae-plugin-sdk-test" || true
}

for loc in $LOCATIONS; do
    # Referrers first, in dependency order: an enrollment names both a bus and
    # a pipeline, a googleApiSource names a bus, and a pipeline names a bus.
    # A referenced resource cannot be deleted.
    for coll in enrollments googleApiSources pipelines messageBuses; do
        for name in $(names_in "$coll" "$loc"); do
            echo "  Deleting $(basename "$name") ($coll in $loc)"
            curl -s -X DELETE -H "Authorization: Bearer ${TOKEN}" \
                "https://eventarc.googleapis.com/v1/${name}" >/dev/null || true
        done
        # The deletes are long-running; the next phase needs the quota back, so
        # wait for the collection to drain rather than racing it.
        for _ in $(seq 1 30); do
            [ -z "$(names_in "$coll" "$loc")" ] && break
            sleep 10
        done
    done
done
echo "clean-eventarc-case: done for $1"
