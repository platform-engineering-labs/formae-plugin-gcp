#!/bin/bash
# © 2025 Platform Engineering Labs Inc.
# SPDX-License-Identifier: FSL-1.1-ALv2
#
# Sync formae-plugin.pkl's minFormaeVersion to the MinFormaeVersion constant
# in the formae plugin SDK currently in use (read from pkg/plugin/version.go).

set -euo pipefail

PLUGIN_DIR=$(go list -m -f '{{.Dir}}' github.com/platform-engineering-labs/formae/pkg/plugin 2>/dev/null || true)
[ -n "$PLUGIN_DIR" ] || exit 0

MIN_VERSION=$(grep 'MinFormaeVersion' "$PLUGIN_DIR/version.go" 2>/dev/null | grep -oE '"[0-9]+\.[0-9]+\.[0-9]+"' | tr -d '"')
[ -n "$MIN_VERSION" ] || exit 0

echo "Updating minFormaeVersion to $MIN_VERSION"
perl -i -pe "s/^minFormaeVersion = .*/minFormaeVersion = \"$MIN_VERSION\"/" formae-plugin.pkl
