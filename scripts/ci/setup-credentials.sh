#!/bin/bash
# © 2025 Platform Engineering Labs Inc.
# SPDX-License-Identifier: Apache-2.0
#
# Setup Credentials Hook
# ======================
# This script is called before running conformance tests to verify
# that cloud provider credentials are properly configured.
#
# Edit this script to check for your provider's required credentials.
# Exit with non-zero status if credentials are not properly configured.
#
# For CI environments (GitHub Actions), credentials are typically
# configured in the workflow file before this script is called.
# This script verifies that the configuration was successful.

set -euo pipefail

# =============================================================================
# CUSTOMIZE THIS SECTION FOR YOUR PROVIDER
# =============================================================================
#
# Example: Check for required environment variables
#
# REQUIRED_VARS=("MY_PROVIDER_API_KEY" "MY_PROVIDER_REGION")
# MISSING_VARS=()
#
# for var in "${REQUIRED_VARS[@]}"; do
#     if [[ -z "${!var:-}" ]]; then
#         MISSING_VARS+=("$var")
#     fi
# done
#
# if [[ ${#MISSING_VARS[@]} -gt 0 ]]; then
#     echo "Error: Missing required environment variables: ${MISSING_VARS[*]}"
#     echo ""
#     echo "For local development, set these variables or source your credentials file:"
#     echo "  export MY_PROVIDER_API_KEY=your-api-key"
#     echo "  export MY_PROVIDER_REGION=us-east-1"
#     echo ""
#     echo "For CI, configure these as secrets in your GitHub workflow."
#     exit 1
# fi
#
# echo "Credentials configured for region: ${MY_PROVIDER_REGION}"

# =============================================================================

echo "setup-credentials.sh: Credentials check not configured"
echo ""
echo "Edit scripts/ci/setup-credentials.sh to verify your provider's credentials."
echo "See comments in this file for an example."
