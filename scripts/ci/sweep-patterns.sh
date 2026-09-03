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

# Every fixture names what it creates "formae-test-<abbrev>-<testRunID>", or
# "formae_test_..." where the API demands underscores. That is now a rule rather
# than a habit: the names were standardised across all 293 fixtures, and every
# sweep below matches exactly that shape.
#
# It was not always so, and the drift cost real money. Sweeps matched whatever
# prefix the fixtures happened to use when they were written - "^formae-plugin-sdk",
# "^formae-test-instance", and two that read "^formae--test" with a doubled hyphen
# and therefore matched nothing at all. Anything named differently was created by
# a run and never collected by one: eight SSL certificates reaching a global cap
# of ten, a hundred and fifty leaked secrets, and Bigtable instances - which hold
# nodes and are billed per node-hour - left behind by every single run of
# bigtable-app-profile, because that fixture calls its instance
# "formae-test-btap-<runID>" and the sweep only looked for "^formae-test-instance".
#
# One shape, one pattern, no per-sweep guessing. "probe" is included so the
# ad-hoc resources a live API probe leaves behind are collected too.
# "plugin" is here for the resources named before the convention landed. A live
# survey found 85 Pub/Sub topics, 21 secrets and 2 networks still carrying
# "formae-plugin-sdk-test-", and a pattern matching only the new shape would have
# left every one of them in the project permanently. It can come out once a sweep
# reports none of them left.
SWEEP_RE="${SWEEP_RE:-^formae[-_](test|probe|plugin)[-_]}"

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
