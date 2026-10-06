#!/usr/bin/env python3
"""Check that sdk/aurora covers the API Nanoleaf documents.

Nanoleaf publishes no machine-readable description of the API, so the saved
copy of its documentation (sdk/aurora-api-specs, see scripts/spec-refresh.py)
is the only list of what the API has. This reads every endpoint and every
effect command out of that page, reads the paths and commands sdk/aurora
sends, and fails if the documentation has one the SDK does not. So "the SDK
covers the documented API" is checked rather than claimed, and a refreshed
page that gains an endpoint fails here until the SDK gains a method.

    scripts/apicheck.py           # the summary, and anything missing
    scripts/apicheck.py --list    # every endpoint and command, and who has it

Standard library only. Run by `make apicheck`.
"""

import argparse
import glob
import os
import re
import sys

ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..")
DOC = os.path.join(ROOT, "sdk", "aurora-api-specs", "wiki", "nanoleaf-light-panels-open-api-documentation.md")
SDK = os.path.join(ROOT, "sdk", "aurora", "*.go")

# What the page documents that sdk/aurora leaves out on purpose, and why. The
# page covers every Nanoleaf product; the SDK is for Light Panels.
LEFT_OUT = {
    "activateScreenMirror": "screen mirroring is for the 4D lightstrip, not Light Panels",
    "setScreenMirrorRhythmMode": "screen mirroring is for the 4D lightstrip, not Light Panels",
    "setScreenMirrorRhythmModeType": "screen mirroring is for the 4D lightstrip, not Light Panels",
    "getScreenMirrorMode": "screen mirroring is for the 4D lightstrip, not Light Panels",
    "getScreenMirrorRhythmMode": "screen mirroring is for the 4D lightstrip, not Light Panels",
}


def normalise(path):
    """/api/v1/auth_token/state/on -> /state/on; the token's place is written
    three ways on the page (auth_token, nothing, and nothing with its slash)."""
    path = path.strip().split("?")[0]
    path = re.sub(r"^/api/v1", "", path)
    path = re.sub(r"^/auth_token", "", path)
    path = re.sub(r"/+", "/", path)
    return path or "/"


def documented():
    with open(DOC, encoding="utf-8") as fh:
        doc = fh.read()

    endpoints = set()
    # each endpoint is a small table: an Endpoint row, then a Request row naming the method
    for m in re.finditer(r"^\| Endpoint \| (\S+) \|\n\| Request \| (GET|PUT|POST|DELETE) \|", doc, re.M):
        endpoints.add((m.group(2), normalise(m.group(1))))
    # the event stream is described in prose, with its request as an example
    for m in re.finditer(r"^(GET|PUT|POST|DELETE) (/api/v1/\S*) HTTP/1\.1", doc, re.M):
        endpoints.add((m.group(1), normalise(m.group(2))))
    if len(endpoints) < 20:
        sys.exit(f"apicheck: only {len(endpoints)} endpoints found in {DOC}: has the page changed how it lists them?")

    # every effect command is shown at least once as "command": "name"; display/add is the page's shorthand for two
    commands = set()
    for m in re.finditer(r'"command"\s*:\s*"([^"]+)"', doc):
        commands.update(m.group(1).split("/"))
    # the summary table names them too, in a row of its own
    for m in re.finditer(r"Command type: ([\w, ]+)", doc):
        commands.update(c.strip() for c in m.group(1).split(","))
    if len(commands) < 8:
        sys.exit(f"apicheck: only {len(commands)} effect commands found in {DOC}: has the page changed how it shows them?")

    return endpoints, commands


def implemented():
    endpoints, commands = set(), set()
    for path in sorted(glob.glob(SDK)):
        if path.endswith("_test.go"):
            continue
        with open(path, encoding="utf-8") as fh:
            src = fh.read()

        for m in re.finditer(r'read\[[\w\[\]]+\]\(ctx, c, "([^"]*)"\)', src):
            endpoints.add(("GET", normalise(m.group(1))))
        for verb, method in (("get", "GET"), ("put", "PUT"), ("query", "PUT"), ("del", "DELETE")):
            for m in re.finditer(r"c\." + verb + r'\(ctx, "([^"]*)"', src):
                endpoints.add((method, normalise(m.group(1))))
        for m in re.finditer(r'c\.exchange\(ctx, http\.Method(\w+), "([^"]*)"', src):
            endpoints.add((m.group(1).upper(), normalise(m.group(2))))
        # the two that build their path by hand: a token request has no token in it, and the stream is not a call
        if re.search(r'http\.MethodPost, path, path, "/new"', src):
            endpoints.add(("POST", "/new"))
        if re.search(r'const path = "/events"', src):
            endpoints.add(("GET", "/events"))

        for m in re.finditer(r'^\s*cmd\w+\s*=\s*"(\w+)"', src, re.M):
            commands.add(m.group(1))

    return endpoints, commands


def main():
    parser = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    parser.add_argument("--list", action="store_true", help="list every endpoint and command, and who has it")
    args = parser.parse_args()

    doc_endpoints, doc_commands = documented()
    our_endpoints, our_commands = implemented()

    missing_endpoints = sorted(doc_endpoints - our_endpoints)
    extra_endpoints = sorted(our_endpoints - doc_endpoints)
    wanted_commands = doc_commands - set(LEFT_OUT)
    missing_commands = sorted(wanted_commands - our_commands)

    if args.list:
        for method, path in sorted(doc_endpoints | our_endpoints, key=lambda e: (e[1], e[0])):
            where = "both" if (method, path) in doc_endpoints & our_endpoints else "docs only" if (method, path) in doc_endpoints else "sdk only"
            print(f"  {method:<6} {path:<34} {where}")
        for name in sorted(doc_commands | our_commands):
            where = "left out" if name in LEFT_OUT else "both" if name in doc_commands & our_commands else "docs only" if name in doc_commands else "sdk only"
            print(f"  write  {name:<34} {where}")
        print()

    print(f"endpoints: {len(doc_endpoints) - len(missing_endpoints)} of the {len(doc_endpoints)} documented have a method in sdk/aurora")
    print(f"effect commands: {len(wanted_commands) - len(missing_commands)} of the {len(wanted_commands)} documented for Light Panels have one")
    if extra_endpoints:
        print("in the SDK and not on the page (controllers answer them all the same): " + ", ".join(f"{m} {p}" for m, p in extra_endpoints))
    left = sorted(set(LEFT_OUT) & doc_commands)
    if left:
        print(f"left out on purpose: {', '.join(left)} ({LEFT_OUT[left[0]]})")

    if missing_endpoints or missing_commands:
        print()
        for method, path in missing_endpoints:
            print(f"MISSING: {method} {path} is documented and sdk/aurora has no method for it")
        for name in missing_commands:
            print(f"MISSING: the effect command {name} is documented and sdk/aurora has no method for it")
        return 1

    return 0


if __name__ == "__main__":
    sys.exit(main())
