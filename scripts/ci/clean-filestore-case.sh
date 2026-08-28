#!/usr/bin/env bash
# © 2025 Platform Engineering Labs Inc.
#
# SPDX-License-Identifier: FSL-1.1-ALv2
#
# Delete the Filestore resources a conformance case leaves behind.
#
# This is the most expensive leak in the suite. A BASIC_HDD instance has a 1 TiB
# minimum and is billed per GiB-month, so one instance left running costs real
# money for as long as nobody notices - and there was no Filestore sweep at all
# before this. The backup and snapshot cases both build an instance as a
# prerequisite, and the harness spares prerequisites on Destroy, so every run
# would leak one.
#
# Snapshots live inside their instance and go with it; backups outlive it and
# must be deleted on their own.
#
# Scoped to ONE case's resources, because the matrix runs the Filestore cases in
# parallel and this is invoked between phases while siblings are still live. A
# project-wide sweep here deleted another job's instance mid-run, and that job
# then failed at Sync with "Inventory should still contain exactly 1 resource
# after sync, got 0" - the resource was genuinely gone, killed by this script.
#
# The argument is the test case name, or "all" for the whole-environment sweep
# in clean-environment.sh, which runs when nothing else does.
set -uo pipefail

CASE="${1:-all}"
case "$CASE" in
    # Each case names its resources after itself; see the fixtures.
    filestore-backup)   PREFIX="formae-test-bkp" ;;
    filestore-snapshot) PREFIX="formae-test-snap" ;;
    filestore-instance) PREFIX="formae-test-fs" ;;
    all)                PREFIX="formae-test-" ;;
    *)
        echo "clean-filestore-case: nothing to do for '${CASE}'"
        exit 0
        ;;
esac

PROJECT="${GCP_PROJECT_ID:-$(gcloud config get-value project 2>/dev/null || true)}"
TOKEN="$(gcloud auth print-access-token 2>/dev/null || true)"
if [ -z "$PROJECT" ] || [ -z "$TOKEN" ]; then
    echo "clean-filestore-case: no project or token, skipping"
    exit 0
fi

api="https://file.googleapis.com/v1"
# "-" asks across every location, so this does not need to know which zone or
# region a case pinned.
all="${api}/projects/${PROJECT}/locations/-"

names_in() { # collection URL -> test-owned resource names
    curl -s -H "Authorization: Bearer ${TOKEN}" "$1" \
        | grep -o "\"name\": *\"[^\"]*\"" \
        | sed -E 's/.*"(projects\/[^"]*)".*/\1/' \
        | grep "${PREFIX}" || true
}

echo "Cleaning GCP filestore resources (${CASE}, prefix ${PREFIX})..."

for backup in $(names_in "${all}/backups"); do
    echo "  Deleting backup: $(basename "$backup")"
    curl -s -X DELETE -H "Authorization: Bearer ${TOKEN}" "${api}/${backup}" >/dev/null || true
done

for instance in $(names_in "${all}/instances"); do
    echo "  Deleting instance: $(basename "$instance")"
    curl -s -X DELETE -H "Authorization: Bearer ${TOKEN}" "${api}/${instance}?force=true" >/dev/null || true
done

# Instance deletes are long-running, and the next phase creates one with the
# same shape. Wait rather than racing it.
for _ in $(seq 1 60); do
    [ -z "$(names_in "${all}/instances")" ] && break
    sleep 10
done
echo "clean-filestore-case: done"
