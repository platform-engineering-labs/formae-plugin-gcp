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
    datastream-*) exec "$here/clean-datastream-case.sh" "$1" ;;
    filestore-*)  exec "$here/clean-filestore-case.sh" "$1" ;;
    security-policy-rule)        PREFIX="formae-plugin-sdk-test-spr-"  KIND=armor ;;
    region-security-policy-rule) PREFIX="formae-plugin-sdk-test-rspr-" KIND=armor ;;
    network-firewall-policy-association)        PREFIX="formae-plugin-sdk-test-nfpa-pol-"  KIND=firewall ;;
    region-network-firewall-policy-association) PREFIX="formae-plugin-sdk-test-rnfpa-pol-" KIND=firewall ;;
    network-firewall-policy-rule)               PREFIX="formae-plugin-sdk-test-nfpr-pol-"  KIND=firewall ;;
    machine-image)                              PREFIX="formae-plugin-sdk-test-mi-"        KIND=vmchain ;;
    spanner-database)                           PREFIX="formae-plugin-sdk-test-spdbi-"     KIND=spanner ;;
    *)
        KIND=network
        ;;
esac

# A case that builds a network leaves it behind: Destroy removes only the
# resource under test. 30 fixtures build one against a project cap of 30, so a
# full matrix runs the cap down on itself and the cases that trip it pass CRUD
# and fail in discovery, where the prerequisite is built again.
#
# The prefix is read out of the fixture rather than written here: it is then
# guaranteed to match what the case actually creates, and 30 hand-copied
# prefixes pointed at live infrastructure is a worse bargain than parsing one.
if [ "${KIND:-}" = "network" ]; then
    fixture="$here/../../testdata/${1}.pkl"
    [ -f "$fixture" ] || { echo "clean-case-prereqs: nothing to do for '${1:-}'"; exit 0; }
    NET_PREFIX=$("$here/network-prefix-for-case.py" "$fixture")

    if [ -z "$NET_PREFIX" ]; then
        echo "clean-case-prereqs: nothing to do for '${1:-}'"
        exit 0
    fi
    # A prefix with no case-specific segment would match every test network in
    # the project, including the ones sibling jobs are using right now. The
    # network case is its own resource under test, so its Destroy already
    # removes it and there is nothing left to reclaim.
    if [ "$NET_PREFIX" = "formae-plugin-sdk-test-" ]; then
        echo "clean-case-prereqs: '${1}' owns no distinct network prefix, skipping"
        exit 0
    fi
    echo "Cleaning networks named ${NET_PREFIX}* ..."
    # A network cannot go before its subnets.
    gcloud compute networks subnets list --format="value(name,region.basename())" 2>/dev/null         | grep "^${NET_PREFIX}" | while read -r n r; do
            echo "  subnet $n ($r)"
            gcloud compute networks subnets delete "$n" --region="$r" --quiet 2>&1 | tail -1 || true
        done
    # Nor before the ranges reserved for private service access on it. Those are
    # global addresses pointing at the network rather than anything under it, so
    # deleting the network first answers RESOURCE_IN_USE_BY_ANOTHER_RESOURCE and
    # the network survives - five accumulated from the memcache case that way,
    # each holding a /16 and a peering.
    #
    # The match is on the network each address points at, not on a name prefix:
    # the range and the network are named differently (mc-range- against
    # mc-net-), so a prefix match would miss it and a looser one would reach
    # ranges belonging to other cases.
    gcloud compute addresses list --global --filter="purpose=VPC_PEERING" --format="value(name,network)" 2>/dev/null | while read -r addr net; do
        case "${net##*/}" in
            "${NET_PREFIX}"*)
                echo "  peering range $addr"
                gcloud compute addresses delete "$addr" --global --quiet 2>&1 | tail -1 || true
                ;;
        esac
    done
    gcloud compute networks list --format="value(name)" 2>/dev/null         | grep "^${NET_PREFIX}" | while read -r n; do
            echo "  network $n"
            gcloud compute networks delete "$n" --quiet 2>&1 | tail -1 || true
        done
    exit 0
fi

# A Spanner instance is billed for as long as it exists, and the database case
# builds one as a prerequisite - which Destroy leaves behind. Left alone until
# the end-of-run sweep, a failed run holds a chargeable instance for the rest of
# the matrix; three of them survived a single afternoon of local runs.
if [ "${KIND:-}" = "spanner" ]; then
    echo "Cleaning Spanner instances named ${PREFIX}* ..."
    # Deleting an instance takes its databases with it.
    gcloud spanner instances list --format="value(name)" 2>/dev/null \
        | grep "^${PREFIX}" | while read -r inst; do
            echo "  instance $inst"
            gcloud spanner instances delete "$inst" --quiet 2>&1 | tail -1 || true
        done
    exit 0
fi

# A machine image is captured from a whole VM, so its case builds a network, a
# subnet, a disk and an instance first. All four outlive the crud phase, and the
# discovery phase then tries to build them again under the same names:
# "The resource 'projects/.../disks/...-mi-disk-...' already exists".
if [ "${KIND:-}" = "vmchain" ]; then
    echo "Cleaning the VM chain named ${PREFIX}* ..."
    # Dependency order: an attached disk cannot be deleted while its instance
    # exists, and a network cannot go before its subnets.
    gcloud compute instances list --format="value(name,zone.basename())" 2>/dev/null \
        | grep "^${PREFIX}" | while read -r n z; do
        echo "  Deleting instance $n ($z)"
        gcloud compute instances delete "$n" --zone="$z" --quiet 2>&1 | tail -1 || true
    done
    gcloud compute disks list --format="value(name,zone.basename())" 2>/dev/null \
        | grep "^${PREFIX}" | while read -r n z; do
        echo "  Deleting disk $n ($z)"
        gcloud compute disks delete "$n" --zone="$z" --quiet 2>&1 | tail -1 || true
    done
    gcloud compute networks subnets list --format="value(name,region.basename())" 2>/dev/null \
        | grep "^${PREFIX}" | while read -r n r; do
        echo "  Deleting subnet $n ($r)"
        gcloud compute networks subnets delete "$n" --region="$r" --quiet 2>&1 | tail -1 || true
    done
    gcloud compute networks list --format="value(name)" 2>/dev/null \
        | grep "^${PREFIX}" | while read -r n; do
        echo "  Deleting network $n"
        gcloud compute networks delete "$n" --quiet 2>&1 | tail -1 || true
    done
    exit 0
fi

# Both collections list regional and global entries together and report the
# region only in a column, so the scope is decided per row rather than by flag.
if [ "$KIND" = "armor" ]; then
    COLLECTION="security-policies"
else
    COLLECTION="network-firewall-policies"
fi

echo "Cleaning ${COLLECTION} named ${PREFIX}* ..."
POLICIES=$(gcloud compute "$COLLECTION" list --format="value(name,region.basename())" 2>/dev/null \
    | grep "^${PREFIX}" || true)

if [ -z "$POLICIES" ]; then
    echo "  none found"
    exit 0
fi

echo "$POLICIES" | while read -r pol region; do
    if [ -n "$region" ]; then
        scope_args=(--region="$region")
        assoc_scope=(--firewall-policy-region="$region")
    else
        scope_args=(--global)
        assoc_scope=(--global-firewall-policy)
    fi

    # A firewall policy with an association attached cannot be deleted, and the
    # association outlives the case when it is the prerequisite rather than the
    # resource under test.
    if [ "$KIND" = "firewall" ]; then
        for assoc in $(gcloud compute network-firewall-policies describe "$pol" "${scope_args[@]}" \
                --format="value(associations[].name)" 2>/dev/null | tr ';,' ' '); do
            [ -z "$assoc" ] && continue
            echo "  Detaching association $assoc from $pol"
            gcloud compute network-firewall-policies associations delete \
                --firewall-policy="$pol" --name="$assoc" "${assoc_scope[@]}" --quiet 2>&1 | tail -1 || true
        done
    fi

    echo "  Deleting $pol (${region:-global})"
    gcloud compute "$COLLECTION" delete "$pol" "${scope_args[@]}" --quiet 2>&1 | tail -1 || true
done
