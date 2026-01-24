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
echo ""
echo "To implement cleanup for your provider, edit this script."
echo "See comments in this file for examples."
echo ""

# Uncomment and modify for your provider:
#
# # AWS - clean up S3 buckets with test prefix
# echo "Cleaning S3 buckets..."
# aws s3api list-buckets --query "Buckets[?starts_with(Name, '${TEST_PREFIX}')].Name" --output text | \
#     xargs -r -n1 aws s3 rb --force s3://
#
# # OpenStack - clean up instances
# echo "Cleaning instances..."
# openstack server list --name "^${TEST_PREFIX}" -f value -c ID | \
#     xargs -r -n1 openstack server delete --wait
#
# # OpenStack - clean up volumes
# echo "Cleaning volumes..."
# openstack volume list --name "^${TEST_PREFIX}" -f value -c ID | \
#     xargs -r -n1 openstack volume delete

echo "clean-environment.sh: Cleanup complete (no-op - not configured)"
