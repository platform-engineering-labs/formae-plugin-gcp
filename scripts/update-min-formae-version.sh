#!/bin/bash
# © 2025 Platform Engineering Labs Inc.
# SPDX-License-Identifier: FSL-1.1-ALv2
#
# Sync formae-plugin.pkl's minFormaeVersion to the MinFormaeVersion constant
# in the formae plugin SDK currently in use (read from pkg/plugin/version.go).
#
# Uses max(SDK floor, declared) semantics: the SDK value is only ever used to
# RAISE minFormaeVersion. If the manifest already declares a higher version we
# keep it — we never silently downgrade below what the plugin declares.

set -euo pipefail

PLUGIN_DIR=$(go list -m -f '{{.Dir}}' github.com/platform-engineering-labs/formae/pkg/plugin 2>/dev/null || true)
[ -n "$PLUGIN_DIR" ] || exit 0

SDK_MIN=$(grep 'MinFormaeVersion' "$PLUGIN_DIR/version.go" 2>/dev/null | grep -oE '"[0-9]+\.[0-9]+\.[0-9]+"' | tr -d '"')
[ -n "$SDK_MIN" ] || exit 0

# Read the version the manifest already declares. Prefer pkl for an exact
# evaluation; fall back to grepping the raw file when pkl is unavailable.
DECLARED=""
if command -v pkl >/dev/null 2>&1; then
  DECLARED=$(pkl eval -x minFormaeVersion formae-plugin.pkl 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' || true)
fi
if [ -z "$DECLARED" ]; then
  DECLARED=$(grep -E '^minFormaeVersion' formae-plugin.pkl | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' || true)
fi
[ -n "$DECLARED" ] || exit 0

# EFFECTIVE = semver-max(SDK_MIN, DECLARED). The SDK value can only raise the
# requirement; it never downgrades below what the manifest already declares.
EFFECTIVE=$(printf '%s\n%s\n' "$SDK_MIN" "$DECLARED" | grep -E '^[0-9]+\.[0-9]+\.[0-9]+$' | sort -t. -k1,1n -k2,2n -k3,3n | tail -1)

if [ "$EFFECTIVE" != "$DECLARED" ]; then
  echo "Raising minFormaeVersion to $EFFECTIVE (sdk=$SDK_MIN, declared=$DECLARED)"
  perl -i -pe "s/^minFormaeVersion = .*/minFormaeVersion = \"$EFFECTIVE\"/" formae-plugin.pkl
else
  echo "Keeping declared minFormaeVersion=$DECLARED (sdk=$SDK_MIN, never downgrade below declared)"
fi
