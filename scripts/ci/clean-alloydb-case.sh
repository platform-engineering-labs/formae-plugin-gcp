#!/usr/bin/env bash
# © 2025 Platform Engineering Labs Inc.
#
# SPDX-License-Identifier: FSL-1.1-ALv2
#
# Delete the AlloyDB clusters one conformance case leaves behind.
#
# ClustersPerProjectPerRegion is 5. Each of the instance, user and backup cases
# builds a cluster as a prerequisite, and the harness spares prerequisites on
# Destroy, so a case leaves one cluster per phase - crud and discovery both. Run
# in parallel across four cases and two phases that needs more slots than the
# quota allows, and every case fails with RESOURCE_EXHAUSTED regardless of the
# code under test.
#
# Each case owns a distinct name prefix, so a job can clear its own leftovers
# between phases without touching a cluster another job is still using.
set -uo pipefail

case "${1:-}" in
    alloydb-cluster)  PREFIX="formae-test-cluster-" ;;
    alloydb-instance) PREFIX="formae-test-inst-cluster-" ;;
    alloydb-user)     PREFIX="formae-test-user-cluster-" ;;
    alloydb-backup)   PREFIX="formae-test-bkp-cluster-" ;;
    *)
        echo "clean-alloydb-case: nothing to do for '${1:-}'"
        exit 0
        ;;
esac

echo "Cleaning AlloyDB clusters named ${PREFIX}* ..."
CLUSTERS=$(gcloud alloydb clusters list --region=- --format="value(name)" 2>/dev/null \
    | grep "/clusters/${PREFIX}" || true)

if [ -z "$CLUSTERS" ]; then
    echo "  none found"
    exit 0
fi

echo "$CLUSTERS" | while read -r cluster; do
    # The region lives only inside the resource path; the region column of
    # "value(name,region)" comes back empty.
    region=$(echo "$cluster" | sed -E 's#.*/locations/([^/]+)/.*#\1#')
    cname=$(basename "$cluster")
    echo "  Deleting AlloyDB cluster: $cname (region: $region)"
    # --force takes the cluster's instances with it.
    gcloud alloydb clusters delete "$cname" --region="$region" --force --quiet 2>&1 | tail -1 || true
done
