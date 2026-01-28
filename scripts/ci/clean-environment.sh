#!/bin/bash
# © 2025 Platform Engineering Labs Inc.
# SPDX-License-Identifier: Apache-2.0
#
# Clean Environment Hook
# ======================
# This script is called before AND after conformance tests to clean up
# test resources in your cloud environment.
#
# Purpose:
# - Before tests: Remove orphaned resources from previous failed runs
# - After tests: Clean up resources created during the test run
#
# The script should be idempotent - safe to run multiple times.
# It should delete all resources matching the test resource prefix.
#
# Test resources typically use a naming convention like:
#   formae-plugin-sdk-test-{run-id}-*
#
# Implementation varies by provider. Examples:
#
# AWS:
#   - List and delete resources with test prefix using AWS CLI
#   - Use resource tagging for easier identification
#
# OpenStack:
#   - Use openstack CLI to list and delete test resources
#   - Clean up in order: instances, volumes, networks, security groups, etc.
#
# Exit with non-zero status only for unexpected errors.
# Missing resources (already cleaned) should not cause failures.

set -euo pipefail

# Prefix used for test resources - should match what conformance tests create
TEST_PREFIX="${TEST_PREFIX:-formae-plugin-sdk-test-}"

echo "clean-environment.sh: Cleaning resources with prefix '${TEST_PREFIX}'"

# GCP - clean up disks with test prefix
echo "Cleaning GCP disks..."
DISKS=$(gcloud compute disks list --filter="name~^formae-plugin-sdk" --format="value(name,zone)" 2>/dev/null || true)
if [ -n "$DISKS" ]; then
    echo "$DISKS" | while read -r disk zone; do
        echo "  Deleting disk: $disk (zone: $zone)"
        gcloud compute disks delete "$disk" --zone="$zone" --quiet 2>/dev/null || true
    done
else
    echo "  No disks found matching prefix 'formae-plugin-sdk*'"
fi

# GCP - clean up networks with test prefix
echo "Cleaning GCP networks..."
NETWORKS=$(gcloud compute networks list --filter="name~^formae-plugin-sdk" --format="value(name)" 2>/dev/null || true)
if [ -n "$NETWORKS" ]; then
    echo "$NETWORKS" | while read -r network; do
        echo "  Deleting network: $network"
        gcloud compute networks delete "$network" --quiet 2>/dev/null || true
    done
else
    echo "  No networks found matching prefix 'formae-plugin-sdk*'"
fi

# GCP - clean up storage buckets with test prefix
echo "Cleaning GCP storage buckets..."
BUCKETS=$(gcloud storage buckets list --filter="name~^formae-plugin-sdk-test" --format="value(name)" 2>/dev/null || true)
if [ -n "$BUCKETS" ]; then
    echo "$BUCKETS" | while read -r bucket; do
        echo "  Deleting bucket: $bucket"
        gcloud storage rm -r "gs://$bucket" --quiet 2>/dev/null || true
    done
else
    echo "  No buckets found matching prefix 'formae-plugin-sdk-test*'"
fi

echo ""
echo "clean-environment.sh: Cleanup complete"
