#!/bin/bash
# Setup GCP Credentials Hook
# ==========================
# This script verifies that GCP credentials are properly configured
# before running conformance tests.
#
# For local development:
#   - Set GCP_CREDENTIALS_FILE to a service account JSON key
#   - Or set GCP_CREDENTIALS_JSON with inline credentials
#   - Or use Application Default Credentials (gcloud auth application-default login)
#
# For CI (GitHub Actions):
#   - Use OIDC with google-github-actions/auth
#   - Or set GCP_CREDENTIALS_JSON secret

set -euo pipefail

echo "Verifying GCP credentials..."
echo ""

# Check for credentials - one of these must be available
if [[ -z "${GCP_CREDENTIALS_FILE:-}" && -z "${GCP_CREDENTIALS_JSON:-}" && -z "${GOOGLE_APPLICATION_CREDENTIALS:-}" ]]; then
    # Check for ADC
    if ! gcloud auth application-default print-access-token > /dev/null 2>&1; then
        echo "ERROR: No GCP credentials configured"
        echo ""
        echo "Set one of the following:"
        echo "  - GCP_CREDENTIALS_FILE (path to service account JSON key)"
        echo "  - GCP_CREDENTIALS_JSON (inline credentials JSON)"
        echo "  - GOOGLE_APPLICATION_CREDENTIALS (path to credentials file)"
        echo ""
        echo "For local development, you can also run:"
        echo "  gcloud auth application-default login"
        exit 1
    fi
fi

# Mask credential values in CI logs (GitHub Actions)
if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
    [[ -n "${GCP_CREDENTIALS_FILE:-}" ]] && echo "::add-mask::${GCP_CREDENTIALS_FILE}"
    [[ -n "${GCP_CREDENTIALS_JSON:-}" ]] && echo "::add-mask::${GCP_CREDENTIALS_JSON}"
    [[ -n "${GOOGLE_APPLICATION_CREDENTIALS:-}" ]] && echo "::add-mask::${GOOGLE_APPLICATION_CREDENTIALS}"
    [[ -n "${GCP_PROJECT_ID:-}" ]] && echo "::add-mask::${GCP_PROJECT_ID}"
    [[ -n "${GCP_PROJECT_NUMBER:-}" ]] && echo "::add-mask::${GCP_PROJECT_NUMBER}"
fi

# Display credential source (without secrets)
if [[ -n "${GCP_CREDENTIALS_FILE:-}" ]]; then
    echo "  Credentials: file (${GCP_CREDENTIALS_FILE##*/})"
elif [[ -n "${GCP_CREDENTIALS_JSON:-}" ]]; then
    echo "  Credentials: inline JSON (${#GCP_CREDENTIALS_JSON} bytes)"
elif [[ -n "${GOOGLE_APPLICATION_CREDENTIALS:-}" ]]; then
    echo "  Credentials: GOOGLE_APPLICATION_CREDENTIALS (${GOOGLE_APPLICATION_CREDENTIALS##*/})"
else
    echo "  Credentials: Application Default Credentials"
fi

echo ""
echo "GCP credentials verified successfully!"
