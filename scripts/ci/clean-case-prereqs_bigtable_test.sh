#!/usr/bin/env bash
# © 2025 Platform Engineering Labs Inc.
#
# SPDX-License-Identifier: FSL-1.1-ALv2
#
# Check that each Bigtable case deletes its own prerequisite instance and only
# its own. Matrix jobs run in parallel against one project, so a case that swept
# a sibling's segment would delete an instance another job is still testing
# against; one that swept the bigtable-instance case's name would delete the
# resource under test. testdata_naming_test.go pins the segments to the
# fixtures - this pins the isolation between them.
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# A gcloud that lists one instance per case plus the bigtable-instance case's
# own resource, and records what it is asked to delete. The names cover all
# three prefixes FIXTURE_PREFIX_RE allows, since a sweep pinned to one variant
# is exactly the failure this guards.
cat > "$tmp/gcloud" <<'STUB'
#!/usr/bin/env bash
if [ "$2" = "instances" ] && [ "$3" = "list" ]; then
  cat <<'NAMES'
formae-test-instance-8f06a19f
formae-test-instance-tbl-8f06a19f
formae-plugin-test-instance-bk-17036332
formae-plugin-sdk-test-instance-mv-b2546cd6
formae-test-instance-cl-aabbccdd
formae-test-btap-d7344f6b
NAMES
  exit 0
fi
if [ "$2" = "instances" ] && [ "$3" = "delete" ]; then
  echo "$4" >> "$DELETED"
fi
STUB
chmod +x "$tmp/gcloud"
export PATH="$tmp:$PATH"

fail=0
check() {
  local case_name="$1" want="$2" got
  export DELETED="$tmp/deleted-$case_name"
  : > "$DELETED"
  "$here/clean-case-prereqs.sh" "$case_name" > /dev/null
  got="$(tr '\n' ' ' < "$DELETED" | sed 's/ *$//')"
  if [ "$got" != "$want" ]; then
    echo "FAIL $case_name: deleted [$got], want [$want]"
    fail=1
  else
    echo "ok   $case_name -> ${want:-(nothing)}"
  fi
}

check bigtable-table              formae-test-instance-tbl-8f06a19f
check bigtable-backup             formae-plugin-test-instance-bk-17036332
check bigtable-materialized-view  formae-plugin-sdk-test-instance-mv-b2546cd6
check bigtable-cluster            formae-test-instance-cl-aabbccdd
check bigtable-app-profile        formae-test-btap-d7344f6b
# The instance IS the resource under test here; Destroy owns it.
check bigtable-instance           ""

exit "$fail"
