#!/usr/bin/env bash
# © 2025 Platform Engineering Labs Inc.
# SPDX-License-Identifier: FSL-1.1-ALv2
# Which names the cleanup tooling treats as ours.
#
# Sourced by scripts/ci/clean-environment.sh (runs in CI) and
# scripts/ci/find-leaks.sh (run by hand). They used to define these separately,
# and the copies drifted: sweeps matched prefixes the fixtures had stopped using,
# so leaked resources piled up unseen - including billable ones. One definition,
# both readers, no drift.
#
# Override any of them from the environment for a one-off run.

# Three prefixes are supported, and all three are live at once:
#
#   formae-plugin-sdk-test-   the default; what a new fixture should use
#   formae-plugin-test-       4 chars shorter, for ids that cannot hold the above
#   formae-test-             12 chars, for the tightest ids of all
#
# plus the underscore forms (formae_plugin_sdk_test_, ...) where the API demands
# them. FIXTURE_PREFIX_RE below is the one definition of that set; every sweep
# builds on it rather than naming a prefix, so no sweep can be pinned to one
# variant while a fixture uses another.
#
# Which fixtures need the shorter forms, and why - a GCP id capped too short for
# 23 characters of prefix plus an 8-character run ID. Each is documented at the
# name in the fixture, and this is the complete list:
#
#   IAM service account accountId   cap 30   iam-service-account{,-replace}.pkl
#   Spanner database id             cap 30   spanner-database{,-replace}.pkl,
#                                            spanner-backup-schedule{,-update}.pkl
#   Bigtable instance id            cap 33   bigtable-*.pkl
#   VPC Access connector name       cap <21  vpcaccess-connector.pkl
#
# The first three are hard arithmetic: 23 + 8 = 31 exceeds 30, and Bigtable's 33
# would fit only by dropping the per-case abbrev, which is what tells you which
# case leaked an instance. Bigtable's 33 is from the Bigtable quotas page ("ID
# length limits": instance 6-33, cluster 6-40, table 1-50, app profile 1-50); the
# other two were already documented at the name before this convention landed.
# The VPC Access rule reads "less than 21 characters, hyphens count as two",
# which the working 25-character name contradicts - either way it is far under
# the 36 the long prefix would need.
#
# SWEEP_RE is the broad catch-all for the whole-environment sweep: it keys off
# "formae[-_]" plus one of test/probe/plugin, so all three prefixes and their
# underscore forms fall in, along with anything left by a hand-run API probe.
# testdata_naming_test.go enforces that fixtures only ever produce those three
# shapes; keep the two files in step.
#
# The drift this guards against cost real money. Sweeps used to match whatever
# prefix the fixtures happened to use when they were written - "^formae-plugin-sdk",
# "^formae-test-instance", and two that read "^formae--test" with a doubled hyphen
# and therefore matched nothing at all. Anything named differently was created by
# a run and never collected by one: eight SSL certificates reaching a global cap
# of ten, a hundred and fifty leaked secrets, and Bigtable instances - which hold
# nodes and are billed per node-hour - left behind by every single run of
# bigtable-app-profile. "probe" is included so the ad-hoc resources a live API
# probe leaves behind are collected too.
SWEEP_RE="${SWEEP_RE:-^formae[-_](test|probe|plugin)[-_]}"

# The three prefixes a fixture is allowed to use, as one ERE alternation. Some
# names are capped too short for the long form (see the table above), so all
# three shapes are live at once and every per-case sweep must accept all three -
# a sweep pinned to one of them silently collects nothing, which is how the
# leaks in the note above happened.
#
# Use it as a prefix and append the case's own segment:
#   PREFIX_RE="${FIXTURE_PREFIX_RE}spr-"
# It is an ERE, so consumers must use "grep -E", not plain grep.
#
# testdata_naming_test.go pins the same three shapes on the fixture side and
# checks that every segment appended here still appears in its fixture.
FIXTURE_PREFIX_RE="${FIXTURE_PREFIX_RE:-formae-(plugin-sdk-|plugin-)?test-}"

# The same three where the API demands underscores (BigQuery, Analytics Hub,
# custom metric descriptors).
FIXTURE_PREFIX_RE_SNAKE="${FIXTURE_PREFIX_RE_SNAKE:-formae_(plugin_sdk_|plugin_)?test_}"

# Names that match SWEEP_RE but must never be deleted. A "formae-" resource is
# not always a leak: formae-byo-cert is a certificate someone installed in July
# and is still in use. Add a name here rather than narrowing SWEEP_RE.
KEEP_RE="${KEEP_RE:-^(formae-byo-cert|formae-tester|formae-tester-nico)([@[:space:]].*)?$}"

# The fixture now names itself "formae-test-sa-<runID>" like everything else, so
# this is the standard shape anchored at the start of the email. The legacy
# "sa-<8hex>@" form is kept so accounts leaked before the rename are still
# collected - narrowing this to a prefix no fixture used is exactly how they
# started leaking. KEEP_RE still guards the identities on top.
SA_SWEEP_RE="${SA_SWEEP_RE:-^(formae[-_](test|plugin)[-_]|sa-[0-9a-f]{8}@)}"
