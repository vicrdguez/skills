#!/usr/bin/env python3
"""Summarize ordinary `go test -json` logs without adding subtest durations."""

import argparse
import collections
import json
from pathlib import Path


def summarize(path, top):
    packages = {}
    tests = []
    started = set()
    cached = False
    with path.open() as source:
        for line in source:
            event = json.loads(line)
            package = event.get("Package")
            action = event["Action"]
            test = event.get("Test")
            if action == "start":
                started.add(package)
            if "(cached)" in event.get("Output", ""):
                cached = True
            if action in ("pass", "fail", "skip"):
                if test is None:
                    packages[package] = (action, event.get("Elapsed", 0))
                elif "/" not in test:
                    tests.append((event.get("Elapsed", 0), package, test, action))
    missing = started - packages.keys()
    empty = not started
    failed = any(status == "fail" for status, _ in packages.values())
    counts = collections.Counter(test[3] for test in tests)
    print(f"\n{path}")
    print(f"Top-level results (including repetitions): {dict(counts)}")
    print("Package elapsed seconds (packages overlap):")
    for package, (status, elapsed) in sorted(
        packages.items(), key=lambda row: row[1][1], reverse=True
    ):
        print(f"  {elapsed:8.3f} {status:4} {package}")
    print("Slowest top-level results (not additive; parents include children/waits):")
    for elapsed, package, test, status in sorted(tests, reverse=True)[:top]:
        print(f"  {elapsed:8.3f} {status:4} {package} {test}")
    if empty:
        print("EMPTY: no package start events")
    if missing:
        print(f"INCOMPLETE: no terminal event for {sorted(missing)}")
    if cached:
        print("CACHED RESULTS: not an uncached timing measurement")
    if failed:
        print("FAILED: not a passing suite measurement")
    return not (empty or missing or cached or failed)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("logs", type=Path, nargs="+")
    parser.add_argument("--top", type=int, default=15)
    args = parser.parse_args()
    valid = [summarize(path, args.top) for path in args.logs]
    return 0 if all(valid) else 1


if __name__ == "__main__":
    raise SystemExit(main())
