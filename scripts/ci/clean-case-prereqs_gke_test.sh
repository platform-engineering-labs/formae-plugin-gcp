#!/usr/bin/env bash
# © 2026 Platform Engineering Labs Inc.
#
# SPDX-License-Identifier: FSL-1.1-ALv2

# Pin the GKE prerequisite cleanup to this case's prefix and dependency order.
# Sibling resources must survive; cluster deletion must precede subnet/network.
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cat > "$tmp/gcloud" <<'STUB'
#!/usr/bin/env bash
case "$*" in
  "container clusters list "*)
    printf '%s\n' \
      'formae-test-gke-run1 us-central1-a' \
      'formae-test-other-run2 us-central1-a' ;;
  "container clusters delete formae-test-gke-run1 "*)
    echo cluster >> "$DELETED" ;;
  "compute networks subnets list "*)
    printf '%s\n' \
      'formae-plugin-sdk-test-gke-subnet-run1 us-central1' \
      'formae-plugin-sdk-test-other-subnet-run2 us-central1' ;;
  "compute networks subnets delete formae-plugin-sdk-test-gke-subnet-run1 "*)
    echo subnet >> "$DELETED" ;;
  "compute networks list "*)
    printf '%s\n' \
      'formae-plugin-sdk-test-gke-net-run1' \
      'formae-plugin-sdk-test-other-net-run2' ;;
  "compute networks delete formae-plugin-sdk-test-gke-net-run1 "*)
    echo network >> "$DELETED" ;;
  *delete*)
    echo "$*" >> "$UNEXPECTED"
    exit 1 ;;
esac
STUB
chmod +x "$tmp/gcloud"
export PATH="$tmp:$PATH" DELETED="$tmp/deleted" UNEXPECTED="$tmp/unexpected"
: > "$DELETED"
: > "$UNEXPECTED"

"$here/clean-case-prereqs.sh" gke-nodepool > /dev/null
got="$(tr '\n' ' ' < "$DELETED" | sed 's/ *$//')"
want="cluster subnet network"
if [ "$got" != "$want" ]; then
  echo "FAIL gke-nodepool cleanup: [$got], want [$want]"
  exit 1
fi
if [ -s "$UNEXPECTED" ]; then
  echo "FAIL gke-nodepool cleanup attempted sibling deletes:"
  cat "$UNEXPECTED"
  exit 1
fi
echo "ok   gke-nodepool cleanup -> $want"
