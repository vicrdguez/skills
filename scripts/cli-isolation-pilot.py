#!/usr/bin/env python3
"""Measure a self-reexec pilot and reproduce its test-cache evidence gap.

Uses an overlay; existing tests and tracked fixtures are never rewritten.
The pilot is deliberately not installed in the normal CLI test package.
"""

import argparse
import json
import os
from pathlib import Path
import statistics
import subprocess
import tempfile
import time


ROOT = Path(__file__).resolve().parents[1]
SCENARIOS = {
    "ledger_test.go": [
        "TestLedgerConfigurationSelectsOneClone",
        "TestLedgerConfigurationRefusals",
        "TestLedgerWorkUsesPrivateDelivery",
    ],
    "delivery_test.go": ["TestDeliveryCLIEndToEnd"],
    "cleanup_ledger_test.go": ["TestCleanupArchivesWholeTerminalProposals"],
    "prose_golden_test.go": ["TestAgentProseGoldens"],
}


def execute(args, environment, log):
    start = time.monotonic()
    result = subprocess.run(
        args, cwd=ROOT, env=environment, stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT, text=True, check=False,
    )
    elapsed = time.monotonic() - start
    log.write_text(result.stdout)
    print(f"{log.name}: exit={result.returncode}, seconds={elapsed:.3f}", flush=True)
    return result.returncode, result.stdout, elapsed


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True,
                        help="new directory for command logs and result.json")
    parser.add_argument("--repetitions", type=int, default=3)
    parser.add_argument("--cache-only", action="store_true")
    options = parser.parse_args()
    require(options.repetitions > 0, "repetitions must be positive")
    output = options.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    environment = os.environ.copy()
    results = {"revision": subprocess.check_output(
        ["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()}

    # Keep the external fixture inside the module so native go test accounts
    # for it, but outside any package's build inputs. Both modes use this path.
    with tempfile.TemporaryDirectory(prefix="cli-isolation-", dir=ROOT / "testdata") as temp:
        temp = Path(temp)
        replacements = {}
        for filename, names in SCENARIOS.items():
            original = ROOT / "cmd/skl" / filename
            contents = original.read_text()
            for name in names:
                declaration = f"func {name}(t *testing.T) {{"
                require(contents.count(declaration) == 1, f"missing scenario: {name}")
                contents = contents.replace(
                    declaration, declaration + "\n\tif isolationPilot(t) { return }", 1)
            replacement = temp / filename
            replacement.write_text(contents)
            replacements[str(original)] = str(replacement)
        replacements[str(ROOT / "cmd/skl/isolation_pilot_test.go")] = str(
            ROOT / "testdata/cli-isolation-pilot/isolation_test.go")
        overlay = temp / "overlay.json"
        overlay.write_text(json.dumps({"Replace": replacements}))
        fixture = temp / "expected.txt"
        environment["SKL_ISOLATION_PILOT_FIXTURE"] = str(fixture)

        def write_fixture(contents):
            fixture.write_text(contents)
            # cmd/go declines to cache files modified less than two seconds
            # ago. Use old, distinct mtimes without sleeping or changing Go's
            # dependency accounting. Contents also differ in size.
            timestamp = time.time() - (120 if contents == "expected\n" else 60)
            os.utime(fixture, (timestamp, timestamp))

        cache_command = ["go", "test", "-overlay", str(overlay), "./cmd/skl",
                         "-run", "^TestIsolationPilotExternalFixture$"]
        cache_results = {}
        for mode in ("native", "isolated"):
            environment["SKL_ISOLATION_PILOT_MODE"] = mode
            write_fixture("expected\n")
            code, _, _ = execute(cache_command, environment, output / f"{mode}-cache-first.log")
            require(code == 0, f"{mode} initial fixture assertion failed")
            code, text, _ = execute(cache_command, environment, output / f"{mode}-cache-hit.log")
            require(code == 0 and "(cached)" in text, f"{mode} did not establish a cached pass")
            write_fixture("wrong external expectation\n")
            code, text, _ = execute(cache_command, environment, output / f"{mode}-cache-changed.log")
            cache_results[mode] = {"changed_exit": code, "changed_cached": "(cached)" in text}
            if mode == "native":
                require(code != 0 and "fixture mismatch" in text,
                        "native execution did not invalidate the changed fixture")
            else:
                require(code == 0 and "(cached)" in text,
                        "expected stale isolated success was not reproduced")
                code, text, _ = execute(cache_command + ["-count=1"], environment,
                                        output / "isolated-uncached-changed.log")
                require(code != 0 and "fixture mismatch" in text,
                        "uncached child did not expose the fixture failure")
        results["cache_probe"] = cache_results
        print("BLOCKER REPRODUCED: self-reexec hides a changed external fixture from go test caching.", flush=True)

        if not options.cache_only:
            binary = temp / "skl.test"
            code, _, _ = execute(
                ["go", "test", "-overlay", str(overlay), "-c", "-o", str(binary), "./cmd/skl"],
                environment, output / "build.log")
            require(code == 0, "pilot binary build failed")
            selected = "^(" + "|".join(name for names in SCENARIOS.values() for name in names) + ")$"
            command = [str(binary), "-test.run=" + selected, "-test.count=1", "-test.parallel=8", "-test.timeout=10m"]
            timings = {"native": [], "isolated": []}
            # Warm the build separately, then alternate modes. No other
            # benchmark is launched by this script during a timed run.
            for repetition in range(options.repetitions):
                for mode in ("native", "isolated"):
                    environment["SKL_ISOLATION_PILOT_MODE"] = mode
                    code, _, elapsed = execute(command, environment, output / f"{mode}-{repetition + 1}.log")
                    require(code == 0, f"{mode} scenario pilot failed")
                    timings[mode].append(elapsed)
            results["seconds"] = timings
            results["median_speedup"] = statistics.median(timings["native"]) / statistics.median(timings["isolated"])
        (output / "result.json").write_text(json.dumps(results, indent=2) + "\n")
        print(json.dumps(results, indent=2), flush=True)


if __name__ == "__main__":
    main()
