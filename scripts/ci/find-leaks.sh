#!/usr/bin/env bash
# © 2025 Platform Engineering Labs Inc.
# SPDX-License-Identifier: FSL-1.1-ALv2
# Find - and optionally delete - leaked conformance resources in the test project.
#
#   scripts/ci/find-leaks.sh            # survey only - lists, deletes NOTHING
#   scripts/ci/find-leaks.sh --delete   # delete what the survey listed
#
# CI sweeps the project on every conformance run; this is for looking at what is
# there now, and for clearing a backlog by hand. It reads the same patterns CI
# does, so what it reports is what CI would collect.
#
# Only names matching the test convention are ever touched:
#   formae-test-*  formae_test_*  formae-probe-*  formae-plugin-*  sa-<8hex>
# The identities and the deliberate certificate are excluded no matter what.
set -uo pipefail

PROJECT="${PROJECT:-${GCP_PROJECT_ID:-development-477117}}"
REGION="${REGION:-${GCP_REGION:-europe-central2}}"

# The same patterns CI sweeps with. Sourced rather than copied: the copies are
# what drifted last time, and a survey that looks for the wrong prefix reports a
# project clean while it is filling up with billable leftovers.
. "$(dirname "$0")/sweep-patterns.sh"
PAT="$SWEEP_RE"
KEEP="$KEEP_RE"
DO_DELETE=0
[ "${1:-}" = "--delete" ] && DO_DELETE=1

# Prove credentials with a call that actually reads a resource. "gcloud projects
# describe" and "gcloud auth print-access-token" both succeed from cache while
# every real request comes back UNAUTHENTICATED, and a survey that cannot see the
# project reports it clean - which is the failure this script exists to catch.
PREFLIGHT_ERR=$(mktemp)
gcloud compute networks list --project="$PROJECT" --limit=1 --format="value(name)" >/dev/null 2>"$PREFLIGHT_ERR"
if grep -qiE "UNAUTHENTICATED|PERMISSION_DENIED|invalid authentication|reauth" "$PREFLIGHT_ERR"; then
  echo "No usable credentials for $PROJECT:"
  sed 's/^/  /' "$PREFLIGHT_ERR" | head -3
  echo "Run: gcloud auth login"
  rm -f "$PREFLIGHT_ERR"; exit 1
fi
rm -f "$PREFLIGHT_ERR"

total_found=0

# label | list command | delete command template ({} = name)
sweep() {
  local label="$1" list_cmd="$2" del_tmpl="$3" out names
  local errf; errf=$(mktemp)
  out=$(eval "$list_cmd" 2>"$errf"); local rc=$?
  # gcloud exits 0 with empty output when it cannot see a collection, so a
  # non-zero status is not the only failure. Read stderr too, or an unreadable
  # service is reported as clean.
  if [ $rc -ne 0 ] || grep -qiE "UNAUTHENTICATED|PERMISSION_DENIED|SERVICE_DISABLED|not enabled|invalid authentication" "$errf"; then
    printf "  %-28s UNREADABLE: %s\n" "$label" "$(head -1 "$errf" | cut -c1-70)"
    rm -f "$errf"; return
  fi
  rm -f "$errf"
  names=$(printf '%s\n' "$out" | grep -E "$PAT" | grep -Ev "$KEEP" || true)
  if [ -z "$names" ]; then printf "  %-28s clean\n" "$label"; return; fi
  local n; n=$(printf '%s\n' "$names" | grep -c .)
  total_found=$((total_found + n))
  printf "  %-28s %s leaked\n" "$label" "$n"
  printf '%s\n' "$names" | sed 's/^/       /'
  if [ "$DO_DELETE" = "1" ]; then
    printf '%s\n' "$names" | while read -r nm; do
      [ -z "$nm" ] && continue
      echo "       deleting $nm"
      eval "${del_tmpl//\{\}/$nm}" >/dev/null 2>&1 || echo "         (delete failed - may need dependents removed first)"
    done
  fi
}

echo "=== BILLABLE (money) ==="
sweep "bigtable instances"  "gcloud bigtable instances list --project=$PROJECT --format='value(name)'" \
                            "gcloud bigtable instances delete {} --project=$PROJECT --quiet"
sweep "filestore instances" "gcloud filestore instances list --project=$PROJECT --format='value(name.basename())'" \
                            "gcloud filestore instances delete {} --project=$PROJECT --region=$REGION --quiet"
sweep "redis instances"     "gcloud redis instances list --project=$PROJECT --region=$REGION --format='value(name.basename())'" \
                            "gcloud redis instances delete {} --project=$PROJECT --region=$REGION --quiet"
# gcloud cannot delete a Cloud SQL instance in this project - it answers
#   Invalid request: Final Backup Retention Days can not be set if
#   enable_final_backup is disabled
# and exits non-zero, so a sweep built on it removes nothing while appearing to
# try. Twenty-one instances accumulated behind that. The REST call has no such
# problem.
sweep "cloud sql instances" "gcloud sql instances list --project=$PROJECT --format='value(name)'" \
                            "curl -sS -o /dev/null -X DELETE -H \"Authorization: Bearer \$(gcloud auth print-access-token)\" https://sqladmin.googleapis.com/v1/projects/$PROJECT/instances/{}"
sweep "spanner instances"   "gcloud spanner instances list --project=$PROJECT --format='value(name)'" \
                            "gcloud spanner instances delete {} --project=$PROJECT --quiet"
sweep "memcache instances"  "gcloud memcache instances list --project=$PROJECT --region=$REGION --format='value(name.basename())'" \
                            "gcloud memcache instances delete {} --project=$PROJECT --region=$REGION --quiet"
sweep "alloydb clusters"    "gcloud alloydb clusters list --project=$PROJECT --region=- --format='value(name.basename())'" \
                            "gcloud alloydb clusters delete {} --project=$PROJECT --region=$REGION --force --quiet"
sweep "privateca pools"     "gcloud privateca pools list --project=$PROJECT --location=$REGION --format='value(name.basename())'" \
                            "gcloud privateca pools delete {} --project=$PROJECT --location=$REGION --quiet"
sweep "compute instances"   "gcloud compute instances list --project=$PROJECT --format='value(name)'" \
                            "gcloud compute instances delete {} --project=$PROJECT --quiet --zone=$REGION-a"

echo
echo "=== FREE but leaked ==="
sweep "pubsub topics"       "gcloud pubsub topics list --project=$PROJECT --format='value(name.basename())'" \
                            "gcloud pubsub topics delete {} --project=$PROJECT --quiet"
sweep "secrets"             "gcloud secrets list --project=$PROJECT --format='value(name)'" \
                            "gcloud secrets delete {} --project=$PROJECT --quiet"
sweep "service accounts"    "gcloud iam service-accounts list --project=$PROJECT --format='value(email)'" \
                            "gcloud iam service-accounts delete {} --project=$PROJECT --quiet"
sweep "sd namespaces"       "gcloud service-directory namespaces list --project=$PROJECT --location=$REGION --format='value(name.basename())'" \
                            "gcloud service-directory namespaces delete {} --project=$PROJECT --location=$REGION --quiet"
sweep "scheduler jobs"      "gcloud scheduler jobs list --project=$PROJECT --location=$REGION --format='value(name.basename())'" \
                            "gcloud scheduler jobs delete {} --project=$PROJECT --location=$REGION --quiet"
sweep "packet mirrorings"   "gcloud compute packet-mirrorings list --project=$PROJECT --format='value(name)'" \
                            "gcloud compute packet-mirrorings delete {} --project=$PROJECT --region=$REGION --quiet"
sweep "compute networks"    "gcloud compute networks list --project=$PROJECT --format='value(name)'" \
                            "gcloud compute networks delete {} --project=$PROJECT --quiet"

echo
if [ "$DO_DELETE" = "1" ]; then
  echo "Deleted what was listed above. Re-run without --delete to confirm it is clean."
  echo "Some deletes fail the first time because a dependent still exists - re-run."
else
  echo "$total_found leaked resource(s) found. Nothing was deleted."
  echo "Re-run with --delete to remove them:   $0 --delete"
fi
