#!/usr/bin/env bash
# © 2025 Platform Engineering Labs Inc.
# SPDX-License-Identifier: FSL-1.1-ALv2
# Decide which conformance test cases a run should exercise.
#
#   push / workflow_dispatch / a PR labelled `full-conformance`
#     -> every case discovered in testdata/
#   any other pull request
#     -> only the cases the PR actually touched
#
# A pull request ran no conformance at all before this: a new or edited fixture
# was unverified until it reached main, and the only early check was dispatching
# debug-conformance.yml by hand with the case names typed out. Scoping to the
# touched cases gives a PR the same evidence the full matrix asks for - CRUD
# *and* discovery green - without paying for the other ~110 live GCP lifecycles.
#
# Deletions are ignored on purpose: a removed fixture has nothing left to run.
# A case's `-update` / `-replace` companion maps back to the case itself, since
# one matrix entry drives the whole lifecycle - the same rule ci.yml already
# uses to keep companions out of the matrix.
#
# Writes `test-cases` (a JSON array) and `count` to $GITHUB_OUTPUT.
#
# Inputs (environment):
#   EVENT_NAME  github.event_name
#   BASE_SHA    github.event.pull_request.base.sha  (pull_request only)
#   FULL_LABEL  "true" when the PR carries the full-conformance label
#   GITHUB_OUTPUT / GITHUB_STEP_SUMMARY  optional; stdout-only when unset
set -euo pipefail

EVENT_NAME="${EVENT_NAME:-push}"
BASE_SHA="${BASE_SHA:-}"
FULL_LABEL="${FULL_LABEL:-false}"

SKIP_FILE=".github/conformance-pr-skip.txt"

emit() {
  local cases="$1" count="$2" reason="$3" skipped="${4:-}"
  if [ -n "${GITHUB_OUTPUT:-}" ]; then
    printf 'test-cases=%s\n' "$cases" >> "$GITHUB_OUTPUT"
    printf 'count=%s\n' "$count" >> "$GITHUB_OUTPUT"
  fi
  if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
    {
      printf '### Conformance scope\n\n%s\n\n' "$reason"
      if [ -n "$skipped" ]; then
        printf 'Skipped per `%s`:\n\n' "$SKIP_FILE"
        printf '%s\n' "$skipped" | sed 's/^/- `/;s/$/`/'
        printf '\n'
      fi
      if [ "$count" -eq 0 ]; then
        printf 'No test cases to run.\n'
      else
        printf '%s test case(s):\n\n' "$count"
        echo "$cases" | jq -r '.[] | "- `\(.)`"'
      fi
    } >> "$GITHUB_STEP_SUMMARY"
  fi
  printf '%s\n' "$reason"
  if [ -n "$skipped" ]; then
    printf 'skipped per %s: %s\n' "$SKIP_FILE" "$(printf '%s' "$skipped" | tr '\n' ' ')"
  fi
  printf 'count=%s\n' "$count"
  echo "$cases" | jq -r 'if length == 0 then "(none)" else join(", ") end'
}

entries() {
  # One name per line, '#' comments and blank lines dropped.
  [ -f "$1" ] || return 0
  grep -vE '^[[:space:]]*(#|$)' "$1" | sed 's/[[:space:]]*$//' | sed '/^$/d'
}

# Every case name ci.yml would run: a top-level testdata/*.pkl that is not an
# -update / -replace companion. testdata/config/ holds shared Pkl, not fixtures.
# On-demand cases are never part of a matrix, whatever the event. They cost
# money to hold or mutate something shared, so they are run by naming them in
# debug-conformance. Excluding them here rather than only from the push path
# keeps a pull request that touches one from starting it too.
ON_DEMAND_FILE="testdata/on-demand-cases.txt"

discovered() {
  find testdata -maxdepth 1 -name '*.pkl' -type f 2>/dev/null \
    | sed 's|^testdata/||;s|\.pkl$||' \
    | grep -vE -- '-(update|replace)$' \
    | sort -u \
    | { local od; od=$(entries "$ON_DEMAND_FILE" | sort -u)
        if [ -n "$od" ]; then comm -23 - <(printf '%s\n' "$od"); else cat; fi; }
}

# A skip entry that names no case does nothing and looks like it does. Fail
# rather than let the two drift apart.
assert_skip_names_exist() {
  local unknown
  unknown=$(comm -23 <(entries "$SKIP_FILE" | sort -u) <(discovered))
  if [ -n "$unknown" ]; then
    echo "::error::$SKIP_FILE names cases that do not exist in testdata/: $(printf '%s' "$unknown" | tr '\n' ' ')" >&2
    exit 1
  fi
}

assert_skip_names_exist

# --- full matrix ------------------------------------------------------------
if [ "$EVENT_NAME" != "pull_request" ] || [ "$FULL_LABEL" = "true" ]; then
  CASES=$(discovered | jq -R . | jq -sc .)
  COUNT=$(echo "$CASES" | jq 'length')
  if [ "$COUNT" -eq 0 ]; then
    echo "::error::no test cases discovered in testdata/" >&2
    exit 1
  fi
  if [ "$FULL_LABEL" = "true" ]; then
    REASON="Full matrix (\`full-conformance\` label)."
  else
    REASON="Full matrix (\`$EVENT_NAME\`)."
  fi
  emit "$CASES" "$COUNT" "$REASON"
  exit 0
fi

# --- pull request: only what changed ---------------------------------------
if [ -z "$BASE_SHA" ]; then
  echo "::error::BASE_SHA is required on a pull_request" >&2
  exit 1
fi

# Three-dot: changes on this branch since it diverged from the base, so an
# unrelated commit landing on the base does not pull extra cases into scope.
# Needs the full history that actions/checkout fetch-depth: 0 provides.
if ! git cat-file -e "${BASE_SHA}^{commit}" 2>/dev/null; then
  echo "::error::base commit $BASE_SHA is not in this checkout - is fetch-depth: 0 set?" >&2
  exit 1
fi

CHANGED=$(git diff --name-only --diff-filter=ACMR "${BASE_SHA}...HEAD" -- 'testdata/*.pkl' || true)

# Git's pathspec glob matches '/', so 'testdata/*.pkl' also catches
# testdata/config/vars.pkl. Only top-level files are fixtures.
NAMES=$(printf '%s\n' "$CHANGED" \
  | sed -n 's|^testdata/\([^/]*\)\.pkl$|\1|p' \
  | sed -E 's/-(update|replace)$//' \
  | sort -u \
  | sed '/^$/d')

if [ -z "$NAMES" ]; then
  emit '[]' 0 'No `testdata/*.pkl` fixtures added or modified - conformance skipped.'
  exit 0
fi

# A name with no fixture behind it would put a matrix entry on the board that
# tests nothing and passes green. Same guard debug-conformance.yml wants for its
# hand-typed input.
MISSING=$(comm -23 <(printf '%s\n' "$NAMES") <(discovered))
if [ -n "$MISSING" ]; then
  echo "::error::changed companion fixture with no base fixture: $(printf '%s' "$MISSING" | tr '\n' ' ') - expected testdata/<name>.pkl" >&2
  exit 1
fi

# Subtract the never-auto-run list. After the existence check, so a typo in the
# skip file cannot mask a genuinely broken fixture name.
SKIPPED=$(comm -12 <(printf '%s\n' "$NAMES") <(entries "$SKIP_FILE" | sort -u))
KEPT=$(comm -23 <(printf '%s\n' "$NAMES") <(entries "$SKIP_FILE" | sort -u))

if [ -z "$KEPT" ]; then
  emit '[]' 0 'Every case this pull request touched is on the never-auto-run list.' "$SKIPPED"
  exit 0
fi

CASES=$(printf '%s\n' "$KEPT" | jq -R . | jq -sc .)
COUNT=$(echo "$CASES" | jq 'length')
emit "$CASES" "$COUNT" 'Test cases added or modified by this pull request.' "$SKIPPED"
