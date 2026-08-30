#!/usr/bin/env python3
# © 2025 Platform Engineering Labs Inc.
#
# SPDX-License-Identifier: FSL-1.1-ALv2
"""Print the network name prefix a conformance fixture creates, or nothing.

The prefix is read out of the fixture rather than written into the cleanup
script: it is then guaranteed to match what the case actually creates, and 30
hand-copied prefixes pointed at live infrastructure is a worse bargain than
parsing one.
"""
import re
import sys


def prefix_of(expr):
    m = re.match(r'"([^"\\]*)\\\(v\.testRunID\)"', expr)
    return m.group(1) if m else ""


def main():
    try:
        s = open(sys.argv[1]).read()
    except OSError:
        return ""

    # Only the network block: a match allowed to run past the closing brace
    # picks up a later resource's name instead.
    block = re.search(r"new network\.Network \{(.*?)\n\}", s, re.S)
    if not block:
        return ""

    m = re.search(r"\n\s*name = (.+)", block.group(1))
    if not m:
        return ""
    expr = m.group(1).strip()

    out = prefix_of(expr)
    if not out and re.match(r"^\w+$", expr):
        # The name is a local. subnetwork and firewall both declare one so the
        # same string can be reused for the network and for a reference to it.
        lm = re.search(r"local\s+" + re.escape(expr) + r'\s*=\s*(".*?")', s)
        if lm:
            out = prefix_of(lm.group(1))
    return out


if __name__ == "__main__":
    print(main())
