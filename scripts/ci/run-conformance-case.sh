#!/usr/bin/env bash
# © 2025 Platform Engineering Labs Inc.
# SPDX-License-Identifier: FSL-1.1-ALv2
# Run one conformance case with the timeouts and phase handling it needs.
#
# Usage: run-conformance-case.sh <test-case> [crud|discovery|both]
#
# Every workflow that runs conformance calls this: ci.yml (push to main),
# nightly.yml (crud only, against formae main) and debug-conformance.yml
# (targeted, on a branch). The per-case timeout table used to be copy-pasted
# into each one and drifted - AlloyDB cases failed the nightly for months on the
# 5 minute default while ci.yml had already allowed 30. One copy, here.
set -uo pipefail

TEST_CASE="${1:?usage: run-conformance-case.sh <test-case> [crud|discovery|both]}"
PHASES="${2:-both}"

CLEAN_PREREQS="$(dirname "$0")/clean-case-prereqs.sh"

# The harness acquires the formae binary and starts an agent before it runs
# anything. Both steps reach the network and both have failed on their own -
# "no available packages for: formae" when the package channel is unreachable,
# "timeout waiting for agent to become ready after 30s" when the agent is slow
# to boot. Neither has touched cloud infrastructure yet, so retrying the phase
# is safe and costs seconds; any other failure is reported as-is on the first
# attempt.
run_make() {
  local log rc attempt
  log="$(mktemp)"
  for attempt in 1 2; do
    make "$@" 2>&1 | tee "$log"
    rc=${PIPESTATUS[0]}
    [ "$rc" -eq 0 ] && return 0
    if [ "$attempt" -lt 2 ] && grep -qE "no available packages for: formae|timeout waiting for agent to become ready" "$log"; then
      echo "::warning::harness setup failed before any test ran, retrying ${TEST_CASE}"
      continue
    fi
    return "$rc"
  done
  return "$rc"
}

# Slow resources need raised timeouts. TIMEOUT sets the per-operation poll
# (FORMAE_TEST_TIMEOUT); the discovery and OOB timeouts are separate env vars
# the SDK reads.
TIMEOUT_ARG=""
case "$TEST_CASE" in
  cloudsql-instance)
    # Cloud SQL provisions an instance per create (5-15 min each). The CRUD
    # lifecycle creates two (initial + OOB-delete re-apply) and discovery
    # creates a third, so allow 30 min per operation.
    TIMEOUT_ARG="TIMEOUT=30"
    export FORMAE_TEST_DISCOVERY_TIMEOUT=30 FORMAE_TEST_OOB_TIMEOUT=30 FORMAE_TEST_OOB_DELETE_TIMEOUT=15
    ;;
  alloydb-cluster|alloydb-instance|alloydb-user|alloydb-backup)
    # An AlloyDB cluster takes 10-25 min to provision, and instance, user and
    # backup each create one first, so every case in this family pays that cost
    # before its own resource starts. The 5m default expires long before the
    # cluster is READY, which is what "timeout waiting for command" on Create
    # means here.
    TIMEOUT_ARG="TIMEOUT=30"
    export FORMAE_TEST_DISCOVERY_TIMEOUT=30 FORMAE_TEST_OOB_TIMEOUT=30 FORMAE_TEST_OOB_DELETE_TIMEOUT=20
    ;;
  spanner-*)
    # A Spanner instance is provisioned compute, and the database and
    # backup-schedule cases each build one as a prerequisite before their own
    # resource starts - 2m30s locally for the database case, before CI load.
    TIMEOUT_ARG="TIMEOUT=15"
    export FORMAE_TEST_DISCOVERY_TIMEOUT=15 FORMAE_TEST_OOB_TIMEOUT=15 FORMAE_TEST_OOB_DELETE_TIMEOUT=15
    ;;
  servicenetworking-connection)
    # PSA connections are async and global (VPC peering); create and especially
    # discovery take longer to surface in inventory than any other resource.
    TIMEOUT_ARG="TIMEOUT=30"
    export FORMAE_TEST_DISCOVERY_TIMEOUT=30 FORMAE_TEST_OOB_TIMEOUT=30 FORMAE_TEST_OOB_DELETE_TIMEOUT=20
    ;;
  eventarc-pipeline|eventarc-message-bus)
    # An Eventarc message bus takes ~5-6 min to create and the pipeline case
    # builds one first, so a single apply runs past the 5m default (6m20s
    # locally for the re-apply, 22m for the whole lifecycle). 15m still was not
    # enough for Update under CI load.
    TIMEOUT_ARG="TIMEOUT=30"
    export FORMAE_TEST_DISCOVERY_TIMEOUT=30 FORMAE_TEST_OOB_TIMEOUT=30 FORMAE_TEST_OOB_DELETE_TIMEOUT=20
    ;;
  url-map|target-http-proxy|target-tcp-proxy|target-grpc-proxy|region-http-lb|global-forwarding-rule)
    # Load-balancer chains create 3-5 dependent resources serially
    # (health-check -> backend-service -> url-map -> proxy -> rule). The CRUD
    # lifecycle already runs ~4m13s on a good day - 84% of the 5m default - so
    # any per-op or operator-startup jitter tips it over.
    TIMEOUT_ARG="TIMEOUT=15"
    export FORMAE_TEST_DISCOVERY_TIMEOUT=15 FORMAE_TEST_OOB_TIMEOUT=15 FORMAE_TEST_OOB_DELETE_TIMEOUT=15
    ;;
  security-policy|region-security-policy|security-policy-rule|region-security-policy-rule)
    # A Cloud Armor policy insert is a slow global/regional operation and the
    # rule cases build one as a prerequisite first; region-security-policy timed
    # out on Create at the 5m default.
    TIMEOUT_ARG="TIMEOUT=15"
    export FORMAE_TEST_DISCOVERY_TIMEOUT=15 FORMAE_TEST_OOB_TIMEOUT=15 FORMAE_TEST_OOB_DELETE_TIMEOUT=15
    ;;
  machine-image)
    # A machine image snapshots every disk of an instance, and the case builds
    # the VM chain (network -> subnet -> disk -> instance) first. It runs ~7m30s
    # end to end locally.
    TIMEOUT_ARG="TIMEOUT=15"
    export FORMAE_TEST_DISCOVERY_TIMEOUT=15 FORMAE_TEST_OOB_TIMEOUT=15 FORMAE_TEST_OOB_DELETE_TIMEOUT=15
    ;;
  instance-group)
    # Creates a VM chain (network -> subnet -> boot-disk -> instance) then the
    # group + synchronous member attach, and the update does removeInstances +
    # setNamedPorts. More serial work than a plain instance.
    TIMEOUT_ARG="TIMEOUT=15"
    export FORMAE_TEST_DISCOVERY_TIMEOUT=15 FORMAE_TEST_OOB_TIMEOUT=15 FORMAE_TEST_OOB_DELETE_TIMEOUT=15
    ;;
  filestore-instance)
    # Filestore BASIC/BASIC_HDD is zonal: its location must be a zone, not a
    # region. The shared GCP_LOCATION secret is a region, so override it with
    # the zone for this test only.
    export GCP_LOCATION="$GCP_ZONE"
    ;;
esac

# Cases whose prerequisite outlives the crud phase need a cleanup between
# phases. Conformance Destroy only removes the resource under test, so the
# prerequisite survives and the discovery phase then either collides with it by
# name (Cloud Armor policies: "The resource ... already exists") or runs out of
# quota (5 AlloyDB clusters per region, 1 Eventarc message bus per region).
needs_prereq_cleanup() {
  case "$TEST_CASE" in
    alloydb-*|eventarc-*|security-policy-rule|region-security-policy-rule|\
    network-firewall-policy-association|region-network-firewall-policy-association|\
    network-firewall-policy-rule|machine-image)
      return 0 ;;
    *)
      return 1 ;;
  esac
}

# shellcheck disable=SC2086 # TIMEOUT_ARG is a deliberate word-split make arg.
if needs_prereq_cleanup; then
  # Run each requested phase separately with a cleanup after it. A failing
  # phase must not abort the script before its cleanup runs, so the status is
  # captured and reported at the end.
  crud_status=0
  discovery_status=0
  if [ "$PHASES" = "crud" ] || [ "$PHASES" = "both" ]; then
    run_make conformance-test-crud-run TEST="$TEST_CASE" PARALLEL=1 $TIMEOUT_ARG || crud_status=$?
    "$CLEAN_PREREQS" "$TEST_CASE"
  fi
  if [ "$PHASES" = "discovery" ] || [ "$PHASES" = "both" ]; then
    run_make conformance-test-discovery-run TEST="$TEST_CASE" PARALLEL=1 $TIMEOUT_ARG || discovery_status=$?
    "$CLEAN_PREREQS" "$TEST_CASE"
  fi
  [ "$crud_status" -eq 0 ] && [ "$discovery_status" -eq 0 ]
else
  case "$PHASES" in
    crud)      run_make conformance-test-crud-run TEST="$TEST_CASE" PARALLEL=1 $TIMEOUT_ARG ;;
    discovery) run_make conformance-test-discovery-run TEST="$TEST_CASE" PARALLEL=1 $TIMEOUT_ARG ;;
    both)      run_make conformance-test-crud-run conformance-test-discovery-run TEST="$TEST_CASE" PARALLEL=1 $TIMEOUT_ARG ;;
    *)         echo "unknown phase: $PHASES" >&2; exit 2 ;;
  esac
fi
