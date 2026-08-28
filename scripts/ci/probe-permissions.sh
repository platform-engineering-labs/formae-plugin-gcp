#!/usr/bin/env bash
# © 2025 Platform Engineering Labs Inc.
#
# SPDX-License-Identifier: FSL-1.1-ALv2
#
# Ask the API which of a list of IAM permissions the caller actually holds.
#
# Run this before writing a batch of resources, not after. A batch was once
# written in full - schemas, provisioners, cleanup script, unit tests - and only
# then found to be denied at create. This answers that in one call, for free,
# and with no side effects.
#
# The answer that matters is CI's, not a laptop's: the workload-identity service
# account CI runs as is broader than a local key, so a local denial is not proof
# a case cannot run. Hence this script, invoked from probe-permissions.yml.
#
# Usage: probe-permissions.sh <comma-or-space-separated permissions>
set -uo pipefail

PERMS_INPUT="${1:?usage: probe-permissions.sh <permissions>}"
PROJECT="${GCP_PROJECT_ID:-$(gcloud config get-value project 2>/dev/null || true)}"
TOKEN="$(gcloud auth print-access-token 2>/dev/null || true)"
if [ -z "$PROJECT" ] || [ -z "$TOKEN" ]; then
    echo "probe-permissions: no project or token" >&2
    exit 1
fi

# One invalid permission name makes the whole request 400 and loses the answer
# for every other permission in it, so probe one at a time. It costs a request
# each and buys an answer that survives a typo.
printf '%s' "$PERMS_INPUT" | tr ', ' '\n\n' | grep -v '^$' | sort -u | while read -r perm; do
    body=$(printf '{"permissions":["%s"]}' "$perm")
    out=$(curl -s -X POST \
        -H "Authorization: Bearer ${TOKEN}" \
        -H "Content-Type: application/json" \
        "https://cloudresourcemanager.googleapis.com/v1/projects/${PROJECT}:testIamPermissions" \
        -d "$body")

    if printf '%s' "$out" | grep -q "\"${perm}\""; then
        printf '  GRANTED  %s\n' "$perm"
    elif printf '%s' "$out" | grep -q 'is not valid for this resource'; then
        # Not a project-level permission - often means the resource is scoped to
        # an organisation or folder, or the name is wrong.
        printf '  INVALID  %s (not a project-level permission)\n' "$perm"
    elif printf '%s' "$out" | grep -q '"error"'; then
        msg=$(printf '%s' "$out" | tr -d '\n' | sed -E 's/.*"message": *"([^"]*)".*/\1/' | cut -c1-90)
        printf '  ERROR    %s (%s)\n' "$perm" "$msg"
    else
        printf '  denied   %s\n' "$perm"
    fi
done
